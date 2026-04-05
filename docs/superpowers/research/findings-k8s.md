# K8s Readiness & Ordering Findings

## Executive Summary

The `sikifanso cluster create` flow installs Cilium and ArgoCD via Helm, then immediately
creates Application CRDs and applies root ApplicationSets — all on a freshly started
single-node k3d cluster. The chain has two structural gaps: (1) `waitForDeployments`
checks only the `Available` condition on three ArgoCD deployments, but `Available=True`
is set before the ArgoCD admission webhook is accepting requests, so `CreateApplications`
can race against webhook readiness; (2) the orchestrator's `watchSingleApp` exits as
soon as `Synced+Degraded` is observed, treating any mid-startup degradation — including
CRD `Establishing` states, image-pull retries, and unbound PVCs — as a permanent failure.
Together these two issues produce false-negative errors for apps that would self-heal
within 2-3 minutes. The root cause is that the readiness model conflates "deployment
replicas are up" with "the cluster is ready to receive application workloads."

---

## Finding 1: ArgoCD Webhook Not Ready When CreateApplications Is Called

**Severity**: High
**Domain**: K8s
**Affected scenario**: `cluster create`

**Root cause hypothesis**: `waitForDeployments` polls `DeploymentAvailable` on
`argocd-server`, `argocd-repo-server`, and `argocd-applicationset-controller`. The
`Available` condition is set by the Deployment controller once `minReadySeconds` elapses
after a pod becomes Ready — but the ArgoCD admission webhook (`argocd-server` exposes it)
can still be initialising its internal TLS cert and gRPC listener after the pod passes its
readiness probe. `CreateApplications` is called immediately after `waitForDeployments`
returns, and the dynamic client `Create` call hits the Kubernetes API server, which routes
through the ArgoCD mutating/validating webhook. If the webhook is not yet bound, the API
server returns a transient `connection refused` or `dial timeout` that surfaces as an
unretried hard error.

**Evidence**:
- `install.go:70-73`: `waitForDeployments` is the only gate before returning control to the
  caller. There is no additional buffer or webhook liveness check.
- `install.go:90-133`: the poll loop checks `deploymentAvailable(dep)` which maps directly
  to `DeploymentAvailable=True` — nothing more.
- `cluster.go:172-192`: `argocd.Install` is followed immediately by `argocd.CreateApplications`
  with no intermediate sleep or readiness probe targeting the webhook endpoint.
- ArgoCD's webhook is served on port 8080 of `argocd-server`. The readiness probe for
  `argocd-server` targets `/healthz` on the same port — so the probe passing does not
  guarantee the Application admission logic is wired up inside the same process.

**Cross-domain flag**: No

**Investigation direction**:
- Add a post-`waitForDeployments` probe that performs a lightweight no-op API call
  through the ArgoCD gRPC client (e.g. `GetVersion`) before calling `CreateApplications`.
- Alternatively, wrap the `Create` call in `CreateApplications` with a retry loop
  (exponential backoff, 3-5 attempts) that distinguishes transient connection errors from
  permanent API errors.

---

## Finding 2: `watchSingleApp` Exits Immediately on First `Synced+Degraded` Event

**Severity**: Critical
**Domain**: K8s
**Affected scenario**: `app enable` with catalog apps that install CRDs or use PVCs

**Root cause hypothesis**: `watchSingleApp` in `orchestrator.go` has two terminal exit
conditions: `Synced+Healthy` (success) and `Synced+Degraded` (failure). During the first
sync of a CRD-heavy Helm chart (e.g. prometheus-stack), ArgoCD completes the `kubectl
apply` of all manifests, marks sync status as `Synced`, but many resources are still
initialising — CRDs are `Establishing`, PVCs are `Pending`, pods are pulling images.
ArgoCD aggregates these into a `Degraded` health status at the Application level. The
orchestrator sees `Synced+Degraded` and exits immediately, fetching the resource tree and
surfacing the error to the user — even though the app would reach `Healthy` in 2-3 minutes
with no intervention.

This is the direct cause of the observed failure: `prometheus-stack` showed
`Synced+Degraded` with Progressing PVCs and an EOF image pull, and the CLI printed
"sync failed: one or more apps unhealthy" even though the cluster recovered on its own.

**Evidence**:
- `orchestrator.go:195-206`: the `Synced+Degraded` branch calls `ResourceTree` and then
  calls `updateFn(r)` followed by `return`. There is no grace period and no re-observation
  window.
- `orchestrator.go:192-194`: `Synced+Healthy` is the only non-error terminal state.
- The `SkipUnhealthy` field in `Request` (types.go:34) exists but is not set by the
  `app enable` command path for the prometheus-stack scenario — its semantics are
  "skip this app entirely", not "keep waiting".
- The comment block at `orchestrator.go:153` explicitly notes this as a known issue:
  "FIX: Don't exit on Degraded immediately. Only exit on Degraded if the app is also
  Synced." — but the fix only guards the `OutOfSync+Degraded` case; it does not introduce
  a stabilisation window for `Synced+Degraded`.

**Cross-domain flag**: No

**Investigation direction**:
- Introduce a configurable stabilisation window (e.g. 30-60 seconds) after the first
  `Synced+Degraded` observation. If the app transitions to `Synced+Healthy` within that
  window, report success. If it remains Degraded or worsens after the window, report
  failure. This is the same pattern used by Flux's `Ready` condition with `retryInterval`.
- Alternatively, classify the Degraded sub-resources: PVCs in `Progressing` and CRDs in
  `Establishing` are expected transient states during first-apply. Exit early only if
  the resource-level health is `Degraded` (not `Progressing`).

---

## Finding 3: CRD Establishment Race During First Helm Apply

**Severity**: High
**Domain**: K8s
**Affected scenario**: `app enable` with CRD-heavy charts (prometheus-stack, cert-manager, etc.)

**Root cause hypothesis**: When a Helm chart includes CRDs (either in the `crds/` directory
or as regular templates), Kubernetes applies them via the API server, which then runs the
CRD validation and schema establishment pipeline asynchronously. The CRD object is created
and immediately visible, but its `Established` condition starts as `False`. ArgoCD syncs
the CRD object successfully (it was accepted by the API server), but its health check
reports `Degraded` because `Established=False`. Any custom resources that reference the
CRD — e.g. `PrometheusAgent`, `Prometheus` — cannot be created until `Established=True`,
so ArgoCD may report their status as `Degraded` as well. On a fresh single-node cluster
there is no spare etcd write bandwidth so CRD establishment can take 10-30 seconds per
CRD even under light load.

The observed log line `CRD/prometheusagents.monitoring.coreos.com health=Degraded CRD is
not established` and `CRD/prometheuses.monitoring.coreos.com health=Progressing Status
conditions not found` are both transient CRD establishment states.

**Evidence**:
- `orchestrator.go:195-206`: the orchestrator exits on `Synced+Degraded` without checking
  whether degraded resources are CRDs in the `Establishing` state.
- ArgoCD's built-in health check for CRD resources checks the `Established` and
  `NamesAccepted` conditions. Until both are `True`, the CRD health is `Progressing`
  (conditions not found) or `Degraded` (explicitly `False`).

**Cross-domain flag**: No

**Investigation direction**:
- Use the `ServerSideApply=true` sync option (already set in `buildApplication`,
  `applications.go:121`) together with ArgoCD's `Replace=true` for CRDs. This alone does
  not fix establishment timing but avoids apply conflicts.
- The correct fix is at the orchestrator layer: do not treat `Progressing` sub-resources
  as a reason to exit. Only sub-resources with `Degraded` health that are not CRDs or PVCs
  should trigger early exit.
- For charts that own CRDs, consider enabling ArgoCD's `CreateNamespace=true` +
  `SkipDryRunOnMissingResource=true` sync options, which tolerate missing CRDs mid-sync.

---

## Finding 4: PVC Binding Is Legitimately Asynchronous on k3d

**Severity**: Medium
**Domain**: K8s
**Affected scenario**: `app enable` with stateful apps (prometheus-stack, Langfuse, ClickHouse)

**Root cause hypothesis**: k3d uses the `local-path-provisioner` (bundled with k3s) as the
default StorageClass. This provisioner creates the backing directory on the host on first
pod scheduling, then binds the PVC. On a freshly started cluster the provisioner pod itself
may still be initialising. PVCs therefore stay in `Pending` → `Bound` for anywhere from
30 seconds to 3 minutes depending on Docker Desktop I/O latency. ArgoCD reports `Pending`
PVCs as `Progressing` health, which contributes to the Application's `Degraded` aggregate
state when combined with any other non-Healthy resource.

**Evidence**:
- The observed failure shows three PVCs in `Progressing` state:
  `PVC/alertmanager-...`, `PVC/prometheus-...`, `PVC/prometheus-stack-grafana`.
- These are `WaitForFirstConsumer` binding mode PVCs — they only bind after a pod is
  scheduled, which itself requires the image to be pulled.
- `orchestrator.go:195`: no special handling for PVCs in `Progressing` state.

**Cross-domain flag**: No

**Investigation direction**:
- PVCs in `Progressing` (not `Degraded`) are not failures. The orchestrator's early-exit
  logic should distinguish `Progressing` from `Degraded` at the sub-resource level.
- For the homelab scenario, consider pre-pulling heavy images or using a local registry
  mirror (k3d supports `--registry-use`) to reduce binding latency cascades.

---

## Finding 5: Image Pull EOF Causes App-Level Degraded

**Severity**: Medium
**Domain**: K8s
**Affected scenario**: `app enable` on a slow or flaky network connection

**Root cause hypothesis**: On macOS with Docker Desktop, image pulls go through the Docker
Desktop VM's network stack, which adds latency and occasional connection resets (manifesting
as EOF during layer streaming). A single pod with `ImagePullBackOff` or `ErrImagePull`
causes the Deployment's rollout to stall. ArgoCD's health assessment for Deployments checks
whether the rollout has progressed; a stalled rollout maps to `Degraded`. Because the
orchestrator exits immediately on `Synced+Degraded`, a transient EOF that kubelet would
retry within 30 seconds causes a permanent CLI failure.

**Evidence**:
- Observed log: `Pod/prometheus-stack-kube-state-metrics-... health=Degraded failed to pull
  image "registry.k8s.io/kube-state-metrics/...:v2.18.0": EOF`.
- Kubelet's image pull retry policy is exponential backoff starting at 10 seconds, capped
  at 5 minutes. The app self-healed after the CLI reported failure, confirming the EOF was
  transient.

**Cross-domain flag**: No

**Investigation direction**:
- The same stabilisation window fix described in Finding 2 covers this case.
- Additionally, consider a local registry mirror or image pre-pull DaemonSet for
  catalog apps that are known to have large images (kube-state-metrics, Grafana).

---

## Finding 6: No Buffer Between `waitForDeployments` and ArgoCD gRPC Readiness

**Severity**: High
**Domain**: K8s
**Affected scenario**: `cluster create` — first `CreateApplications` call after fresh install

**Root cause hypothesis**: `waitForDeployments` in `install.go` checks the Kubernetes
`Deployment.Available` condition. A pod is `Ready` when its readiness probe passes. For
`argocd-server`, the readiness probe is an HTTP GET to `/healthz`. However, ArgoCD's gRPC
service — used by the `grpcclient` — starts on the same pod and listens on port 8080. The
gRPC server is initialised after the HTTP handlers and requires loading the ArgoCD
application cache, establishing the in-cluster Kubernetes informer watches, and registering
the webhook handlers. These steps complete asynchronously after the pod's readiness probe
passes. The `syncAfterMutation` middleware (referenced in CLAUDE.md) and `CreateApplications`
use the gRPC client immediately after `Install` returns, and can hit the gRPC server before
it has finished initialising.

**Evidence**:
- `install.go:70-73`: control returns to `cluster.go:172` as soon as `waitForDeployments`
  passes, with no gRPC-level health check.
- `cluster.go:177-192`: `CreateApplications` uses the dynamic Kubernetes client (not gRPC),
  but the API server routes through the ArgoCD webhook for `argoproj.io` resources.
- `grpcclient/client.go` (not read in detail but referenced): the gRPC client connects at
  call time; a connection refused at that moment yields an opaque error returned from
  `SyncApplication` or `GetApplication`.

**Cross-domain flag**: No

**Investigation direction**:
- After `waitForDeployments`, add a gRPC ping probe: call `argocd.GetVersion()` (or
  `ServerVersion`) in a retry loop (e.g. up to 30 seconds, 2-second intervals) before
  proceeding to `CreateApplications`. This is the same technique the ArgoCD CLI itself
  uses in its `--grpc-web-root-path` healthcheck path.
- Alternatively, wrap `CreateApplications` in a retry loop since it already handles
  `AlreadyExists` idempotently.

---

## Finding 7: All Root ApplicationSets Start Concurrently Without Sequencing

**Severity**: Medium
**Domain**: K8s
**Affected scenario**: `cluster create` on a constrained single-node k3d host

**Root cause hypothesis**: After `CreateApplications` registers the Cilium and ArgoCD
Applications, `gitops.ApplyRootApp` applies all three root ApplicationSets
(`root-app.yaml`, `root-catalog.yaml`, `root-agents.yaml`). ArgoCD begins reconciling all
of them simultaneously. On a single Docker Desktop VM (typically 2-4 CPUs, 4-8 GB RAM),
this means Cilium (CNI), ArgoCD (large chart), and any catalog entries already marked
`enabled: true` all compete for image pull bandwidth, container runtime scheduling, etcd
write throughput, and CPU during the same 2-3 minute window. This amplifies all the
transient failures described in Findings 3-5.

**Evidence**:
- `cluster.go:195-197`: `gitops.ApplyRootApp` is called with no sequencing against the
  ArgoCD and Cilium Application syncs.
- CLAUDE.md describes the triple-track model: root-app, root-catalog, root-agents all
  start from a single `ApplyRootApp` call.
- The observed failure had three PVCs and a pod all in transient states simultaneously,
  consistent with parallel startup contention.

**Cross-domain flag**: No

**Investigation direction**:
- Enforce startup ordering: wait for the Cilium Application to reach `Synced+Healthy`
  before applying root-catalog and root-agents ApplicationSets.
- After Cilium is healthy, wait for ArgoCD's own Application to reach `Synced+Healthy`
  before enabling catalog entries, since ArgoCD's CRDs and webhooks may be reinstalled by
  its own Application sync.
- This requires the orchestrator to support a `WaitForApps` primitive that blocks until
  a named set of Applications are healthy before releasing the next phase.

---

## Finding 8: `kubeconfig` Resolved from Default File Path in Every API Call

**Severity**: Low
**Domain**: K8s
**Affected scenario**: All — any multi-cluster user

**Root cause hypothesis**: Both `waitForDeployments` (`install.go:91`) and
`CreateApplications` (`applications.go:44`) call
`clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)`, which reads
`~/.kube/config` and uses whatever context is currently active. sikifanso calls
`k3dclient.KubeconfigGetWrite` with `UpdateCurrentContext: true` (`cluster.go:149-153`)
immediately before, so the context switch is likely correct for a single-cluster workflow.
However, in a multi-cluster scenario (two sikifanso clusters side-by-side), if the user
switches context between cluster creation steps, or if another process changes the context,
the wrong cluster receives the ArgoCD install. There is also no error if the current context
does not match the cluster being created.

**Evidence**:
- `install.go:91-98` and `applications.go:44-52`: both independently resolve kubeconfig
  from the recommended home file with no cluster-name validation.
- `cluster.go:148-154`: kubeconfig is written and context updated, but the kubeconfig
  object is discarded rather than being threaded through to subsequent API calls.

**Cross-domain flag**: No

**Investigation direction**:
- Pass the `*rest.Config` (or a `clientcmd.ClientConfig` bound to the specific cluster name)
  from `KubeconfigGetWrite` through `Install` and `CreateApplications` as a parameter,
  rather than re-reading the global kubeconfig. This eliminates the TOCTOU race between
  context-switch and API calls.
