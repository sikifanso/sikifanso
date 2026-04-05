# ArgoCD Deployment Research Report
**Date**: 2026-04-04
**Input**: 5 specialist findings documents (ArgoCD, K8s, Go, Homelab, AppWorkloads)

---

## Executive Summary

The most critical problem in the sikifanso CLI is that `watchSingleApp` exits immediately
on the first `Synced+Degraded` event and classifies it as a hard failure — but ArgoCD
routinely produces `Synced+Degraded` for 5–60 seconds after a Helm sync completes while
CRDs are establishing, PVCs are binding, and images are being pulled. This single issue is
the direct cause of the observed `app enable postgresql` failure; the app self-healed
minutes after the CLI reported failure. A second systemic defect amplifies the damage:
the `cluster create` post-profile sync uses the wrong operation (`OpSync` with no
`ReconcileFn`) against newly-enabled catalog apps that do not yet exist as ArgoCD
Application CRs, meaning profile deployments silently skip the apps they are meant to
deploy. On the catalog/infrastructure side, the Langfuse bundled ClickHouse and MinIO
sub-charts carry no resource constraints, making their footprint opaque and uncontrolled;
the `agent-full` profile's total memory request (~6.6 GiB declared, plus ~2 GiB infra
overhead) risks OOM evictions on 8-16 GiB hosts. The three highest-priority investigation
tasks are: **(T1) implement a Degraded grace-period window in `watchSingleApp`**, **(T2)
fix the cluster-create post-profile sync to use `OpEnable` with AppSet reconciliation**,
and **(T3) add resource constraints to Langfuse's ClickHouse and MinIO sub-charts**.

---

## Problem Catalog

---

### P1: watchSingleApp Exits Immediately on Transient Synced+Degraded
**Severity**: Critical
**Domains**: ArgoCD, K8s, Go
**Affected scenarios**: `app enable` with any CRD-heavy chart (prometheus-stack,
cert-manager, temporal); `app enable` with auto-enabled dependencies; any app with PVCs or
slow image pulls on a constrained host.

**Description**: When `watchSingleApp` receives a watch event showing
`SyncStatus=Synced && Health=Degraded`, it immediately fetches the resource tree, calls
`updateFn`, and returns. The parent `watchApps` maps `Health=Degraded` to `ExitFailure`,
which `syncAfterMutation` surfaces to the user as `"sync failed: one or more apps
unhealthy"`. ArgoCD routinely produces `Synced+Degraded` as a transient state immediately
after a Helm sync because: (a) CRDs pass through `Degraded` ("CRD is not established") for
5–30 seconds while the API server registers them; (b) PVCs in `WaitForFirstConsumer` mode
are `Progressing` (contributing to aggregate `Degraded`) until a pod is scheduled and the
image is pulled; (c) a single transient `ErrImagePull`/EOF causes the Pod and its parent
Deployment to be `Degraded` even though kubelet will retry within 10–30 seconds. The fix
comment at `orchestrator.go:152-153` acknowledges the `OutOfSync+Degraded` case was
repaired but the `Synced+Degraded` transient window was not. The `SkipUnhealthy` field in
`Request` (types.go:34) does not solve this — its semantics are "skip this app entirely",
not "wait for it to stabilise".

**Root cause**: `orchestrator.go:195` — exits unconditionally on
`Health == "Degraded" && SyncStatus == "Synced"` with no minimum observation window or
resource-type carve-outs. No grace period, no re-observation logic.

**Impact**: Every first-time deployment of a CRD-heavy chart produces a false-negative
CLI failure. The cluster self-heals but the user is left believing the operation failed,
requiring a manual retry or cluster recreate.

---

### P2: Cluster Create Post-Profile Sync Uses Wrong Operation Against Non-Existent Apps
**Severity**: Critical
**Domains**: Go
**Affected scenarios**: `cluster create --profile <any>` — the final sync step after
`profile.Apply`.

**Description**: After `profile.Apply` commits catalog YAML to the gitops repo,
`cluster_create.go:115–129` calls `client.ListApplications()` and passes the result to
`orch.SyncAndWait` with `OpSync` (the zero-value default). `OpSync` calls
`SyncApplication` directly on each returned app name — it does not trigger AppSet
reconciliation. At this point, the newly-enabled catalog apps exist only as YAML files in
`catalog/*.yaml`; the ArgoCD ApplicationSet controller has not yet reconciled them into
Application CRs. `ListApplications` therefore returns either an empty list (silent no-op)
or only the root ApplicationSets themselves. When the list is empty, the `if len(names) > 0`
guard silently skips the entire sync. The user sees the cluster creation complete
successfully while none of the profile apps were actually deployed.

**Root cause**: `cluster_create.go:115–129` — `ListApplications` + `OpSync` does not
model the ArgoCD lifecycle correctly. Newly-enabled catalog apps require AppSet
reconciliation (annotation patch) before they exist as Application CRs. The correct
operation is `OpEnable` with a `ReconcileFn` pointing to the catalog ApplicationSet
annotation trigger — identical to what `syncAfterMutation` does via `middleware.go:111–121`.

**Impact**: Profile deployments via `cluster create` are silently incomplete. Users who
rely on `--profile agent-dev` to get a working cluster will find apps are not deployed.

---

### P3: Catalog ApplicationSet Has No Automated SyncPolicy — Silent Stall Window
**Severity**: Critical
**Domains**: ArgoCD
**Affected scenarios**: `app enable` / `app disable` for any catalog entry; any case where
the manual `SyncApplication` call in `waitForAppear` is rejected.

**Description**: `root-catalog.yaml` defines no `automated` block in its Application
template. The sole sync driver for catalog apps is the manual `SyncApplication` call in
`waitForAppear` (`orchestrator.go:241`). If that call arrives while ArgoCD is processing
another reconcile cycle, or if the request is rejected (another operation already in
flight), the app remains `OutOfSync` indefinitely — there is no background self-heal to
recover it. This is inconsistent: `root-app.yaml` and `root-agents.yaml` both set
`automated.selfHeal: true` / `prune: true`. The failed sync error is swallowed at Debug
level: "sync trigger after appear failed (may already be syncing)".

**Root cause**: `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml:72-83` — `syncPolicy`
block contains only `syncOptions` and `retry`; no `automated` key. Compare with
`root-app.yaml:37-40` and `root-agents.yaml:37-40` which both set `automated.selfHeal: true`.

**Impact**: If the `SyncApplication` call races with an ArgoCD reconcile (common during
cluster bootstrap under load), the catalog app stalls `OutOfSync` until a user manually
re-runs `app enable` or waits for the 180-second reconcile interval.

---

### P4: ArgoCD Webhook/gRPC Not Ready When CreateApplications Is Called After Install
**Severity**: High
**Domains**: K8s, ArgoCD
**Affected scenarios**: `cluster create` — the `CreateApplications` call immediately after
`argocd.Install` returns.

**Description**: `waitForDeployments` in `install.go` polls `DeploymentAvailable` on three
ArgoCD deployments and returns as soon as all pass. `Available=True` is set once a pod's
readiness probe passes (HTTP GET `/healthz` on port 8080). However, the ArgoCD admission
webhook (which the Kubernetes API server routes through for `argoproj.io` resources) and
the gRPC service initialise asynchronously after the pod passes its readiness probe — they
require loading the application cache, establishing informer watches, and registering TLS
certs. `CreateApplications` is called immediately after `waitForDeployments` returns and
issues a dynamic Kubernetes `Create` call that routes through the webhook. If the webhook
is not yet bound, the API server returns `connection refused` or `dial timeout` as an
unretried hard error, halting cluster creation.

**Root cause**: `install.go:70-73` — no gRPC-level readiness check after
`waitForDeployments`. `cluster.go:172-192` — `argocd.Install` is followed immediately by
`argocd.CreateApplications` with no intermediate probe.

**Impact**: Intermittent `cluster create` failures that are timing-dependent and hard to
reproduce. More common on slow hosts or under Docker Desktop resource contention.

---

### P5: post-profile Sync and cluster create use Hardcoded ~/.kube/config
**Severity**: High
**Domains**: ArgoCD, K8s
**Affected scenarios**: Multi-cluster setups; `cluster create` on a machine with an active
non-sikifanso kubeconfig context; `app enable` where `CreateApplications` is called.

**Description**: Both `waitForDeployments` (`install.go:91-98`) and `CreateApplications`
(`applications.go:44-52`) independently call
`clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)`, which reads
`~/.kube/config` and uses whatever context is currently active. `appsetreconcile/reconcile.go`
correctly accepts `*rest.Config` as a parameter. `cluster.go:148-154` writes the kubeconfig
and updates the current context immediately before these calls, so the race window is small
in a single-cluster workflow. In a multi-cluster scenario, if the user has switched context
or another process has changed it between the kubeconfig write and the API calls,
`CreateApplications` silently targets the wrong cluster.

**Root cause**: `applications.go:44` — `clientcmd.RecommendedHomeFile` is hardcoded; no
cluster name or session path is threaded through. `install.go:91` — same pattern. The
`*rest.Config` returned by `KubeconfigGetWrite` is discarded at `cluster.go:148-154`
instead of being passed to subsequent calls.

**Impact**: In a multi-cluster workflow, ArgoCD Application CRDs can be created in the
wrong cluster with no error or warning. Silent data misrouting.

---

### P6: RollingSync Tier Ordering Declared in ApplicationSet but Ignored by CLI and Absent from Most Apps
**Severity**: High
**Domains**: ArgoCD, AppWorkloads
**Affected scenarios**: `app enable` with multiple apps spanning tiers; `cluster create`
with any profile that includes both `0-operators` and `2-services` tier apps.

**Description**: `root-catalog.yaml` is configured with `strategy.type: RollingSync`
across three tiers: `0-operators`, `1-data`, `2-services`. ArgoCD will not begin syncing
tier-1 until all tier-0 apps are `Healthy`. However, the CLI's `watchApps`
(`orchestrator.go:96-115`) launches a goroutine per app concurrently with no
tier-awareness. Tier-1 and tier-2 apps held back by ArgoCD's RollingSync will appear as
`OutOfSync/Unknown` to the CLI's watchers, which maps to `ExitTimeout` — even though
ArgoCD is doing exactly the right thing. Compounding this, 14 of 19 catalog apps have no
`tier` or `dependsOn` field at all, so they are applied in an unordered batch alongside
tier-0 operators. Specifically: `valkey` (a shared service that langfuse depends on) has
no tier; `litellm-proxy` has no tier despite runtime coupling to langfuse and ollama;
`alloy` has no tier despite hardcoded loki/tempo endpoint dependencies; `loki` and `tempo`
have no tier even though alloy depends on them.

**Root cause (two-part)**:
1. `orchestrator.go:96-115` — concurrent per-app goroutines, no tier sequencing.
2. Catalog YAML: `valkey.yaml`, `litellm-proxy.yaml`, `alloy.yaml`, `loki.yaml`,
   `tempo.yaml` (and 9 others) — missing `tier` and `dependsOn` fields.

**Impact**: Concurrent startup is fragile for apps with boot-time service dependencies.
The CLI reports spurious timeouts for apps legitimately held in the RollingSync queue. On
a constrained node, all apps compete for resources simultaneously rather than in ordered
waves.

---

### P7: agent-minimal Profile Missing valkey — Langfuse Fails Redis Connection on Startup
**Severity**: High
**Domains**: AppWorkloads
**Affected scenarios**: `cluster create --profile agent-minimal`; `sikifanso app enable langfuse`
without first enabling valkey.

**Description**: The `agent-minimal` profile enables `[litellm-proxy, langfuse, cnpg-operator,
postgresql]`. Langfuse's values file configures `redis.deploy: false` and
`redis.host: valkey.storage.svc.cluster.local`, meaning it expects the shared valkey
instance to be running. valkey is not in the `agent-minimal` app list, so it will not be
deployed. Langfuse will start, fail its Redis connection check, and either refuse requests
or produce errors on every trace submission. The `langfuse.yaml` catalog entry also does
not list `valkey` in its `dependsOn`, so ArgoCD will not enforce the ordering.

**Root cause**: `internal/profile/profile.go:22-25` — agent-minimal app list omits valkey.
`catalog/langfuse.yaml:10-12` — `dependsOn: [cnpg-operator, postgresql]` does not include
valkey. `catalog/values/langfuse.yaml:29-34` — `redis.deploy: false`, wired to shared valkey.

**Impact**: `agent-minimal` is a documented entry profile; its deployment failure on
langfuse is a user-visible regression affecting the simplest on-boarding path.

---

### P8: Langfuse Bundled ClickHouse and MinIO Have No Resource Constraints
**Severity**: High
**Domains**: Homelab, AppWorkloads
**Affected scenarios**: Any profile including langfuse (`agent-minimal`, `agent-dev`,
`agent-safe`, `agent-full`); `app enable langfuse`.

**Description**: The langfuse values file sets resources only for the main application
container (256Mi request / 1Gi limit). Two bundled sub-charts — ClickHouse
(`clickhouse.deploy: true`) and MinIO (`s3.deploy: true`) — inherit upstream defaults with
no homelab overrides. ClickHouse defaults to a 2 GiB memory request with no limit; under
analytical query load it rises to 4-12 GiB. MinIO typically allocates 256–512 MiB. Neither
has limits set, so a single langfuse deployment can silently consume most of the cluster's
available memory — and was the direct cause of the Langfuse OOM failure noted in the spec
("ClickHouse requesting ~12 GB on a single homelab node"). Additionally, there is no
standalone `clickhouse` or `minio` catalog entry, so these services cannot be shared across
apps or independently managed.

**Root cause**: `sikifanso-homelab-bootstrap/catalog/values/langfuse.yaml:37-58` —
`clickhouse.deploy: true` and `s3.deploy: true` with no `resources:` stanza under either
sub-chart. ClickHouse upstream chart default: 2 GiB memory request, no limit.

**Impact**: Langfuse deployment causes OOM evictions on nodes with less than 6-8 GiB free.
On a constrained single-node cluster, enabling langfuse can destabilise all other apps
running on the same node.

---

### P9: Presidio Analyzer Memory Limit Causes OOM on Startup
**Severity**: High
**Domains**: AppWorkloads, Homelab
**Affected scenarios**: `app enable presidio`; any profile including presidio (`agent-safe`,
`agent-full`).

**Description**: The Presidio analyzer container loads spaCy `en_core_web_lg` at startup.
This model requires 700-800 MiB of RSS before processing a single request. The values file
sets `limits.memory: 512Mi` for the analyzer, which is less than the model's minimum
working set. The container will be OOM-killed before it finishes initialising. The anonymizer
(128Mi request / 256Mi limit) is fine as it is purely rule-based.

**Root cause**: `sikifanso-homelab-bootstrap/catalog/values/presidio.yaml` — analyzer
`limits.memory: 512Mi`. Microsoft Presidio Docker image bundles `en_core_web_lg` by
default; `en_core_web_lg` loaded RSS: 700-800 MiB.

**Impact**: Presidio will never successfully start. Any profile or user that enables
presidio will see a CrashLoopBackOff that cannot self-heal regardless of wait time.

---

### P10: gRPC Stream Fallback pollOnce Provides a Single Stale Snapshot
**Severity**: Medium
**Domains**: ArgoCD, Go
**Affected scenarios**: Any `watchSingleApp` call where the gRPC stream drops mid-sync
(network hiccup, ArgoCD pod restart, Docker bridge connection-tracking timeout).

**Description**: When the gRPC watch stream closes unexpectedly, `watchSingleApp` falls
back to a single `pollOnce` call and returns. If `GetApplication` catches the app in any
transient intermediate state (`OutOfSync/Progressing`, mid-sync), that state is recorded
as the final result, producing a spurious timeout or false health report. A zeroed result
(when `GetApplication` also fails) has `Health == ""` and `SyncStatus == ""`, which falls
into the `default` branch of `watchApps` exit-code logic as `ExitTimeout` with no
indication to the user that the watch failed mid-stream. The asymmetry with
`waitForDisappear` is notable: that function uses `pollUntilGone` (a loop), not
`pollOnce`.

**Root cause**: `orchestrator.go:167-170` — `case event, ok := <-eventCh:` with
`if !ok { o.pollOnce(...); return }`. `orchestrator.go:320-336` — `pollOnce` calls
`updateFn(Result{App: name})` (zeroed) on error.

**Impact**: Stream drops on slow or loaded clusters cause silent misclassification of sync
results. The gRPC connection has no keep-alive (`client.go:44-85`), making stream drops
more likely on Docker's internal bridge network.

---

### P11: waitForAppear Polling Starts After First Tick — Always Waits ≥5s
**Severity**: Medium
**Domains**: ArgoCD, Go
**Affected scenarios**: `app enable` with fast-reconciling ApplicationSets; every
`OpEnable` path.

**Description**: `waitForAppear` creates a ticker and immediately enters a `select` that
blocks until the first tick fires (`orchestrator.go:219-237`). The tick interval is
`DefaultPollInterval = 5 * time.Second` (`types.go:12`). This means the function always
waits at least 5 seconds before checking whether the Application CR exists — even if the
AppSet controller has already reconciled and created it within 1 second of the annotation
patch. Additionally, `Trigger()` in `appsetreconcile/reconcile.go:45-56` patches the
annotation and returns immediately with no confirmation that the controller acted on it;
`waitForAppear` never checks for annotation removal (which is the controller's signal that
reconciliation ran), so it may start polling before the controller has had any chance to
respond.

**Root cause**: `orchestrator.go:215-235` — ticker-first loop with no initial `GetApplication`
check. `appsetreconcile/reconcile.go:45-56` — `Trigger` returns immediately after PATCH;
no poll for annotation removal.

**Impact**: Adds unnecessary latency to every `app enable`. Masks annotation patch failures:
if `Trigger()` errors, execution continues and `waitForAppear` blocks for its full timeout
with no clear message that the root cause was a failed annotation patch.

---

### P12: SyncApplication Called After waitForAppear Without Operation-State Guard
**Severity**: Medium
**Domains**: ArgoCD
**Affected scenarios**: `app enable` for `root` and `agents` ApplicationSet apps (which
have `automated.selfHeal: true`).

**Description**: `waitForAppear` calls `SyncApplication` unconditionally as soon as
`GetApplication` returns a non-NotFound result (`orchestrator.go:241`). For `root` and
`agents` ApplicationSets, the Application is created with `automated.selfHeal: true /
prune: true`, meaning ArgoCD's controller may already have started an automatic sync.
Sending a manual `SyncApplication` while ArgoCD's internal operation is `Running` produces
a gRPC error ("another operation is already in progress"), currently swallowed at Debug
level. The manual sync request can also disrupt a carefully-ordered CRD-before-workload
sync sequence, causing resource-ordering failures.

**Root cause**: `orchestrator.go:241-243` — `SyncApplication` called unconditionally; error
is Debug-logged with message "may already be syncing".

**Impact**: Redundant sync triggers on auto-sync apps; potential disruption of in-flight
syncs for complex charts.

---

### P13: Error Messages Discard All Available Context
**Severity**: Medium
**Domains**: Go
**Affected scenarios**: Any sync failure or timeout — interactive terminal and CI pipelines.

**Description**: `syncAfterMutation` receives the full `[]grpcsync.Result` slice with
per-resource health details (app name, SyncStatus, Health, Message, Resources), but the
error messages returned to the user contain only static strings:
`"sync failed: one or more apps unhealthy"` or
`"sync timed out: apps may still be reconciling"`. The user is given no indication of
which app failed, what its health/sync state was, or which resources are degraded. The
`printSyncResults` call has the data but produces stderr output before the error is
returned — in CI or when output is piped, this detail is lost.

**Root cause**: `middleware.go:136-151` — error construction uses only static strings;
`results` is ignored. `grpcsync/types.go:39-46` — `Result` carries `App`, `SyncStatus`,
`Health`, `Message`, `Resources` — all discarded.

**Impact**: Users and CI pipelines cannot determine why a sync failed without grepping
detailed logs. Increases support burden and time-to-diagnosis.

---

### P14: watchApps Default Branch Conflates Progressing and Unknown Exit States
**Severity**: Medium
**Domains**: Go
**Affected scenarios**: Any sync that times out while an app is `Progressing`, `Unknown`,
or `Suspended`.

**Description**: The `watchApps` exit-code logic (`orchestrator.go:127-143`) has three
cases: success (`Synced+Healthy` or `Deleted`), explicit failure (`Degraded` or `Missing`),
and a `default` branch that maps everything else to `ExitTimeout`. The `default` covers
`Progressing`, `Unknown`, `Suspended`, and any future ArgoCD health states. An app that is
`Unknown` because ArgoCD lost contact with the cluster is indistinguishable from an app
that is legitimately `Progressing` mid-sync. Both produce `ExitTimeout` and the same
"sync timed out" message. `Unknown` state typically indicates a connectivity or cluster
health problem that warrants a louder and different signal.

**Root cause**: `orchestrator.go:134-143` — `default` case maps to `ExitTimeout`.
`middleware.go:149-150` — both timeout cases produce identical messages.

**Impact**: Users cannot distinguish transient progress delays from cluster connectivity
failures, leading to incorrect remediation attempts.

---

### P15: Spinner Suffix Is a Data Race Under Concurrent Watches
**Severity**: Medium
**Domains**: Go
**Affected scenarios**: `app enable` with multiple apps syncing concurrently (the common
case for auto-deps).

**Description**: `watchApps` spawns one goroutine per app (`orchestrator.go:96-115`). Each
goroutine calls `req.OnProgress`, which directly assigns `s.Suffix` in the outer
`syncAfterMutation` closure (`middleware.go:114-120`). The spinner library does not protect
`Suffix` with a mutex in its public API — the background spinner goroutine reads `s.Suffix`
on every tick while the watch goroutines write it. Running `go test -race` on any
`app enable` with two or more apps will surface this as a data race on `s.Suffix`. Beyond
the race, the suffix is overwritten by whichever goroutine fires last, so earlier progress
from other apps is silently lost.

**Root cause**: `middleware.go:103` and `middleware.go:114-120` — single spinner with
unprotected `Suffix` written from multiple goroutines simultaneously.

**Impact**: Data race; undefined behavior in Go. In practice the spinner shows only the
most recently reported app, dropping progress information for all others.

---

### P16: ReconcileFn Error Is Swallowed, Causing Full-Timeout Block on Patch Failure
**Severity**: Medium
**Domains**: Go
**Affected scenarios**: `app enable` with a transient AppSet annotation patch failure.

**Description**: `ReconcileFn` (the AppSet annotation patch) is called once at the start
of `OpEnable`/`OpDisable` (`orchestrator.go:43-55`). If the patch fails,
`SyncAndWait` logs a warning and continues into `watchApps`. `watchApps` then blocks for
the full context timeout (typically 5-10 minutes) waiting for apps that will never appear
because the ApplicationSet was never told to reconcile them. There is no retry and no early
exit.

**Root cause**: `orchestrator.go:43-46` — `ReconcileFn` error is only logged as `Warn`,
not returned. `appsetreconcile/reconcile.go:48-52` — `Trigger` returns an error on patch
failure; the error is silently downgraded.

**Impact**: A transient RBAC hiccup or API server blip causes the entire `app enable`
operation to block silently for its full timeout before reporting failure.

---

### P17: All Root ApplicationSets Start Concurrently on cluster create — Resource Contention
**Severity**: Medium
**Domains**: K8s
**Affected scenarios**: `cluster create` on a constrained single-node k3d host (< 16 GiB RAM).

**Description**: After `CreateApplications`, `gitops.ApplyRootApp` applies all three root
ApplicationSets simultaneously (`cluster.go:195-197`). ArgoCD reconciles all of them at
once, meaning Cilium, ArgoCD itself, and any catalog entries already `enabled: true` all
compete for image pull bandwidth, container runtime scheduling, etcd write throughput, and
CPU during the same 2-3 minute window. This amplifies every transient failure described in
P1 — CRD establishment races, PVC binding delays, and image pull EOFs all occur
concurrently rather than sequentially, increasing the probability of a false-negative CLI
exit.

**Root cause**: `cluster.go:195-197` — `gitops.ApplyRootApp` is called with no sequencing
against the Cilium and ArgoCD Application syncs.

**Impact**: On an 8 GiB host, simultaneous startup of all enabled apps routinely produces
OOM pressure and cascading `Synced+Degraded` states that trigger P1's false-negative.

---

### P18: ResourceTree Used for Failure Diagnostics Misses Sync Error Messages
**Severity**: Medium
**Domains**: ArgoCD
**Affected scenarios**: Any failed sync — the `✗` resource detail shown in terminal output.

**Description**: When `watchSingleApp` detects `Synced+Degraded`, it calls `ResourceTree`
(`orchestrator.go:197`) to populate `r.Resources` for display. `ResourceTree` maps
`tree.Nodes` to `ResourceStatus`, but `tree.Nodes` have no `SyncStatus` field — only
health. The more useful per-resource sync errors (e.g. "hook failed", "resource already
exists") are in `app.Status.Resources`, which is returned by `GetApplication` and already
mapped by `toResourceStatuses`. The `ResourceTree` call is also more expensive (full graph
fetch) and leaves the `Status` field of `ResourceStatus` empty, so any unhealthy-but-synced
resources are silent in terminal output.

**Root cause**: `grpcclient/applications.go:154-181` — `ResourceTree` maps `tree.Nodes`
with no `Status` (sync) field. `grpcclient/types.go:40-48` — `ResourceStatus.Status` is
left empty by the ResourceTree path.

**Impact**: Failure diagnostics show health errors but not sync errors, making triage
harder and causing engineers to miss the most actionable failure reason.

---

### P19: gRPC Connection Has No Keep-Alive — Silent Dead Streams Under Docker Bridge
**Severity**: Medium
**Domains**: ArgoCD
**Affected scenarios**: Long-running watches (slow Helm charts, large resource counts,
cluster under load).

**Description**: The gRPC client is created with plain `apiclient.ClientOptions` and no
additional `DialOptions`. No `keepalive.ClientParameters` are configured. In a k3d cluster
running inside Docker, the bridge network's connection-tracking can silently drop idle TCP
connections (no events for > ~20s). When this happens, `stream.Recv()` blocks until the
next event or context cancellation rather than returning an error immediately — the `!ok`
channel-close branch is never triggered. The `WatchApplication` channel buffer of 16 is
also not monitored for exhaustion.

**Root cause**: `grpcclient/client.go:44-85` — no `grpc.WithKeepaliveParams`,
no `grpc.WithBlock`, no `grpc.WithReturnConnectionError`.
`grpcclient/applications.go:125-148` — tight `Recv` loop; silently-dead stream blocks
indefinitely.

**Impact**: On a Docker Desktop host under memory pressure, long syncs can silently stall
mid-watch with no fallback to polling, making the CLI appear hung.

---

### P20: agent-full Profile Exceeds Safe Memory Envelope for 8-16 GiB Hosts
**Severity**: Medium
**Domains**: Homelab
**Affected scenarios**: `cluster create --profile agent-full`; enabling all 19 catalog apps
simultaneously.

**Description**: The `agent-full` profile enables all 19 catalog apps. Declared memory
requests total ~6.6 GiB (excluding Langfuse bundled sub-charts). Adding Cilium (~300 MiB),
ArgoCD (~500 MiB), CoreDNS + k3d overhead (~150 MiB), and Langfuse's unconstrained
ClickHouse (~500 MiB minimum), total system pressure reaches 8-9 GiB on declared requests
alone. On an 8 GiB host the OOM killer will evict pods under any load spike. Total PVC
allocation reaches 89 GiB, which may exhaust local-path storage on developer laptops.

**Root cause**: `internal/profile/profile.go:29-37` — all 19 apps listed. No preflight
memory check before applying the profile.

**Impact**: `agent-full` is unreliable on the primary target hardware class (developer
laptop, 8-16 GiB RAM, Docker Desktop).

---

### P21: Temporal Helm Values Only Set Top-Level server.resources — Four Services Go Unconstrained
**Severity**: Medium
**Domains**: Homelab, AppWorkloads
**Affected scenarios**: `app enable temporal`; any profile including temporal.

**Description**: The Temporal Helm chart deploys four separate server components
(frontend, history, matching, worker), each as its own Deployment. The values file sets
only a single `server.resources` block. In practice, each Temporal service pod runs
independently and the chart defines separate `frontend`, `history`, `matching`, `worker`
top-level keys each accepting their own `resources` and `replicaCount`. Without per-service
overrides, actual cluster consumption is likely 4× the declared 256 MiB request.
Additionally, Temporal values do not include `elasticsearch.enabled: false`, and the chart
may render Elasticsearch schema jobs by default, which will CrashLoop on a cluster without
Elasticsearch.

**Root cause**: `catalog/values/temporal.yaml:4-13` — single `server.resources` block;
no per-service breakdown. No `elasticsearch.enabled: false`.

**Impact**: Temporal's actual footprint is significantly larger than reported. On a
constrained node, the four Temporal service pods will compete for unallocated resources
and may trigger the OOM killer.

---

### P22: Prometheus PVC Oversized at 20 GiB — Combined PVC Footprint Reaches 89 GiB
**Severity**: Medium
**Domains**: Homelab
**Affected scenarios**: Any profile including prometheus-stack; `agent-full`; `agent-dev`.

**Description**: Prometheus is configured with `retention: 7d` and a 20 GiB storage claim.
For a single-node homelab scraping 20-40 pods, 7-day retention typically consumes 1-3 GiB.
The 20 GiB figure is likely copied from a production template. Combined with Ollama's 20 GiB
model PVC, Loki's 10 GiB, Tempo's 10 GiB, PostgreSQL's 10 GiB, Qdrant's 10 GiB,
AlertManager's 5 GiB, Grafana's 2 GiB, and Valkey's 2 GiB, total PVC allocation reaches
89 GiB for the full profile. Additionally, there is no `retentionSize` guard — Prometheus
will silently fill the full PVC regardless of the time-based retention policy.

**Root cause**: `catalog/values/prometheus-stack.yaml:23-25` — `storage: 20Gi` with no
`retentionSize`.

**Impact**: Developer laptops with 256-512 GB SSDs may not comfortably accommodate 89 GiB
of PVC allocations alongside Docker images and OS files.

---

### P23: Agent Sandbox ResourceQuota Conflates Requests and Limits — Forces Guaranteed QoS
**Severity**: Medium
**Domains**: Homelab, AppWorkloads
**Affected scenarios**: All agent sandbox deployments via `sikifanso agent create`.

**Description**: The ResourceQuota template hard-sets `requests.cpu`, `limits.cpu`,
`requests.memory`, and `limits.memory` all to the same value (`agent.cpu` and
`agent.memory`). This forces every container in the agent namespace to set requests equal
to limits to satisfy the quota, placing all agent pods in `Guaranteed` QoS class. This
prevents Burstable QoS (request < limit) for orchestration agents that have variable CPU
usage patterns. A 500m CPU / 512 MiB memory quota also leaves no headroom for auxiliary
containers (sidecars, init containers) within the same namespace.

**Root cause**: `sikifanso-agent-template/templates/resourcequota.yaml:9-14` — `limits.cpu`
and `requests.cpu` both set to `{{ .Values.agent.cpu }}`.
`sikifanso-agent-template/values.yaml:4-8` — single values used for both requests and
limits quota.

**Impact**: Agent workloads requiring burst CPU (e.g., local inference calls) are throttled
to their request ceiling. Init containers count against the quota, causing scheduling
failures for multi-container agent pods.

---

### P24: cnpg-operator Has No Resource Requests or Limits
**Severity**: Low
**Domains**: Homelab
**Affected scenarios**: All profiles (cnpg-operator is a dependency of postgresql which is
in every profile).

**Description**: The cnpg-operator values file sets `replicaCount: 1`, `crds.create: true`,
and `config.clusterWide: true`, but contains no `resources:` stanza. All other
operator-style apps in the catalog (external-secrets, opa) have explicit resources set.
Without bounds, an operator bug or high reconciliation load could consume unbounded memory.

**Root cause**: `catalog/values/cnpg-operator.yaml` — no `resources:` key.

**Impact**: Unbounded resource consumption on a single-node cluster with no QoS isolation.
Low risk in practice (the CNPG operator is well-behaved), but inconsistent with catalog
conventions.

---

### P25: nemo-guardrails Has No LLM Endpoint Configured — App Will Fail to Apply Any Rail
**Severity**: Low
**Domains**: AppWorkloads
**Affected scenarios**: `app enable nemo-guardrails`; `agent-safe` and `agent-full` profiles.

**Description**: NeMo Guardrails is a Python server that invokes an LLM to evaluate safety
rules. The values file sets no `llm.endpoint` or `openai.*` wiring. Without an LLM endpoint
configured, NeMo Guardrails will start but will fail to apply any LLM-backed rail
(responding with errors on any guarded request). There is also no `dependsOn: [litellm-proxy]`
in the catalog entry, so the intended upstream LLM proxy may not be ready when nemo-guardrails
starts.

**Root cause**: `catalog/values/nemo-guardrails.yaml` — no LLM endpoint configuration.
`catalog/nemo-guardrails.yaml` — no `dependsOn`.

**Impact**: nemo-guardrails deploys successfully but is non-functional out of the box.
Confusing UX for first-time users of the agent-safe profile.

---

## Cross-Domain Insights

---

### CDX-1: Synced+Degraded False-Negative Has Three Compounding Causes Across Domains
**Domains involved**: ArgoCD, K8s, Go

**The interaction**: Three independently-observed findings all describe the same user-visible
failure (`"sync failed: one or more apps unhealthy"` for a self-healing app), but from
different angles:

- **ArgoCD** (Finding 1): `orchestrator.go:195` exits on the first `Synced+Degraded`
  watch event with no observation window. ArgoCD's health model does not distinguish
  "resources still initialising" from "permanent failure."
- **K8s** (Findings 2, 3, 4, 5): CRDs pass through `Establishing/Degraded` for 5-30s;
  PVCs in `WaitForFirstConsumer` mode are `Progressing` until pod scheduling; image pull
  EOFs on Docker Desktop cause `Degraded` Deployments that kubelet retries within 30s.
  All of these are normal first-sync behaviour on a single-node k3d cluster.
- **Go** (Finding 1): The exit point is in `watchApps` (`orchestrator.go:134-138`) which
  maps `Health == "Degraded"` to `ExitFailure` with no grace period. The `// FIX` comment
  in the source acknowledges this but the implementation remains.

**Combined fix direction**: A single `DegradedGracePeriod` field (default 60s) in the
`Request` type, checked in `watchSingleApp`: on the first `Synced+Degraded` event, start a
timer rather than returning. If the app transitions to `Synced+Healthy` before the timer
fires, report success. If the timer expires while still `Degraded`, call `ResourceTree` and
report failure. This single change resolves the confirmed production defect with minimal
structural impact. Optionally, classify degraded sub-resource types (CRD `Establishing`,
PVC `Progressing`) as explicitly transient to exit even faster once all non-transient
resources are healthy.

---

### CDX-2: RollingSync Tier Ordering Is Partially Declared and Entirely Ignored by the CLI
**Domains involved**: ArgoCD, AppWorkloads

**The interaction**: `root-catalog.yaml` declares `strategy.type: RollingSync` with three
ordered tiers, which is architecturally correct — operators first, data services second,
application services third. However, 14 of 19 catalog apps have no `tier` label, so they
fall into an unordered batch alongside tier-0 operators. The apps that are missing tiers
are not arbitrary: `valkey` is a shared service that `langfuse` depends on at boot time;
`litellm-proxy` hard-codes `LANGFUSE_HOST` and `ollama` endpoints; `alloy` hardcodes loki
and tempo endpoints. These missing labels mean the RollingSync strategy provides no
protection for the most fragile dependencies. Independently, the CLI's `watchApps` watches
all apps concurrently with no tier awareness, so even for apps that do have `tier` labels,
the CLI reports spurious `ExitTimeout` for tier-2 apps that are intentionally held by
ArgoCD until tier-0 is healthy.

**Combined fix direction**: Two changes required:
1. **Catalog YAML**: add `tier` and `dependsOn` to the 14 apps missing them (see T6 for
   the full list). Priority: `valkey` → tier-1 data; `loki`, `tempo` → tier-1 data;
   `alloy` → tier-2 with `dependsOn: [loki, tempo]`; `litellm-proxy` → tier-2 with
   `dependsOn: [langfuse, ollama]`; `nemo-guardrails` → tier-2 with
   `dependsOn: [litellm-proxy]`.
2. **CLI**: modify `watchApps` to be tier-aware — start watching tier-0 apps first, then
   release tier-1 watchers only after all tier-0 apps reach `Synced+Healthy`. The
   `catalog.Entry.Tier` field is already populated at the CLI layer; it needs to be
   threaded into the `Request` type and used to sequence goroutine launches.

---

### CDX-3: Langfuse ClickHouse and MinIO Bundling Creates an Unconstrained OOM Hazard
**Domains involved**: Homelab, AppWorkloads

**The interaction**: The Homelab agent found that the Langfuse sub-charts have no resource
constraints, noting ClickHouse defaults to 2 GiB RAM with no limit. The AppWorkloads agent
found that there is no standalone `clickhouse` or `minio` catalog entry, so these services
are permanently bundled — they cannot be shared, independently scaled, or separately
constrained. Together these create a situation where: (a) any deployment of langfuse
silently provisions unconstrained ClickHouse and MinIO alongside it; (b) there is no
observable catalog entry to modify or replace; (c) the memory footprint is opaque and can
fill all available RAM on a constrained node before any OOM protection triggers.

**Combined fix direction**: Two parallel changes:
1. **Immediate**: add `clickhouse.resources` and `s3.resources` overrides to
   `catalog/values/langfuse.yaml`. For ClickHouse: `requests.memory: 512Mi`,
   `limits.memory: 1Gi`; tune `clickhouse.settings.max_memory_usage` to match. For
   MinIO: `requests.memory: 128Mi`, `limits.memory: 256Mi`.
2. **Strategic**: create standalone `clickhouse` and `minio` catalog entries
   (`tier: 1-data`) and update langfuse to point at them via `clickhouse.host` and
   `s3.endpoint` value overrides. The `project_langfuse_deps.md` memory note aligns with
   this direction.

---

### CDX-4: Hardcoded ~/.kube/config Across ArgoCD Install and Application Creation
**Domains involved**: ArgoCD, K8s

**The interaction**: The ArgoCD agent found `applications.go:44` uses
`clientcmd.RecommendedHomeFile` (the global kubeconfig) rather than the session's
cluster-specific config. The K8s agent found the same pattern in `install.go:91-98`. Both
call sites independently re-resolve the kubeconfig from `~/.kube/config` rather than
accepting the `*rest.Config` that `cluster.go:148-154` already has available immediately
after `KubeconfigGetWrite`. The pattern in `appsetreconcile/reconcile.go:31-38` — which
correctly accepts `*rest.Config` as a parameter — shows the right approach is already
established in the codebase.

**Combined fix direction**: A single refactor: change both `Install` and `CreateApplications`
signatures to accept `*rest.Config` as a parameter, and pass the config obtained from
`KubeconfigGetWrite` at `cluster.go:148-154` through to both call sites. This eliminates
two independent TOCTOU races between context-switch and API calls, and makes both functions
testable without a real cluster.

---

## Investigation Task List

---

### T1: Implement Degraded Grace Period in watchSingleApp
**Priority**: Critical
**Related problems**: P1, CDX-1
**Domain**: Go (ArgoCD orchestrator)
**Files/configs to examine**:
  - `internal/argocd/grpcsync/orchestrator.go` — `watchSingleApp` function, lines 154-210;
    the `Synced+Degraded` exit branch at line 195; the `watchApps` exit-code mapping at
    lines 127-143
  - `internal/argocd/grpcsync/types.go` — `Request` struct (add `DegradedGracePeriod
    time.Duration`); `DefaultPollInterval` constant
  - `cmd/sikifanso/middleware.go` — `syncAfterMutation` — where `Request` is constructed;
    what `SkipUnhealthy` is currently set to

**Specific questions to answer**:
  1. Does adding `DegradedGracePeriod` to the `Request` struct break any existing callers
     that construct `Request` directly (not through `SyncAndWait`)? List all construction
     sites.
  2. Should the grace period be per-app (each app gets its own timer) or global (the first
     Degraded app starts one shared timer)? What is the correct behaviour when app A goes
     Degraded, recovers, and then app B goes Degraded?
  3. What should `watchSingleApp` do if `ctx` expires while the grace period timer is still
     running — return `ExitTimeout` or `ExitFailure`?
  4. Is it safe to call `ResourceTree` only after the grace period expires, or does the
     result become stale if the app recovered and then degraded again?
  5. Confirm whether `OperationState.Phase == "Succeeded" && Health == "Degraded"` (sync
     completed, resources still converging) is reliably distinguishable from
     `OperationState.Phase == "Failed" && Health == "Degraded"` (sync itself failed) via
     `GetApplication` — if so, document the fast-exit path for genuine failures.

**Definition of done**: A `DegradedGracePeriod` is wired into `watchSingleApp` such that
the confirmed failure scenario (prometheus-stack `Synced+Degraded` during CRD
establishment, PVC binding, image pull retry) no longer produces `ExitFailure`. A new test
case in `grpcsync/` simulates a sequence of
`Synced+Degraded → ... → Synced+Healthy` events and asserts success.

---

### T2: Fix cluster create Post-Profile Sync to Use OpEnable with AppSet Reconciliation
**Priority**: Critical
**Related problems**: P2
**Domain**: Go (cluster_create.go)
**Files/configs to examine**:
  - `cmd/sikifanso/cluster_create.go` — lines 115-129; the `ListApplications` +
    `SyncAndWait` block; confirm what is in `names` at this point in execution
  - `cmd/sikifanso/middleware.go` — `syncAfterMutation` function (lines 100-155);
    how it constructs `Request` with `OpEnable` and `ReconcileFn`
  - `internal/argocd/grpcsync/orchestrator.go` — `OpEnable` path (lines 57-80);
    `OpSync` path (lines 57-64)
  - `internal/profile/profile.go` — `Apply` return value and what app names it has access
    to after committing YAML

**Specific questions to answer**:
  1. Does `profile.Apply` return or have access to the list of app names it enabled? If not,
     what is the cleanest way to extract them (parse the returned catalog entries, or add a
     return value to `Apply`)?
  2. What `ReconcileFn` should be used — the same annotation-patch trigger used by
     `syncAfterMutation`? Is there already a constructor for it, or must it be constructed
     inline?
  3. Should the post-profile sync use `OpEnable` or `OpSync` for apps that may already exist
     as Application CRs from a previous partial run? Confirm the idempotency contract.
  4. Is the timeout used in `cluster_create.go` for this sync appropriate given that it may
     need to wait for multi-tier RollingSync sequencing?

**Definition of done**: `cluster create --profile agent-dev` on a fresh cluster results in
all profile apps being deployed (not silently skipped). A manual test or integration test
confirms that the `ReconcileFn` is called and that `waitForAppear` is reached for each
newly-enabled app.

---

### T3: Add Resource Constraints to Langfuse Bundled ClickHouse and MinIO
**Priority**: Critical
**Related problems**: P8, CDX-3
**Domain**: Homelab
**Files/configs to examine**:
  - `sikifanso-homelab-bootstrap/catalog/values/langfuse.yaml` — lines 37-58;
    `clickhouse.deploy: true` and `s3.deploy: true` sections; confirm the exact Helm value
    paths for sub-chart resource overrides
  - Langfuse Helm chart source (referenced chart: `langfuse/langfuse`) — check
    `values.yaml` schema for `clickhouse.resources` and `s3.resources` nesting
  - `project_langfuse_deps.md` memory note — confirms strategic direction for standalone
    catalog entries

**Specific questions to answer**:
  1. What is the exact Helm value path to set resource requests/limits for the bundled
     ClickHouse sub-chart? (e.g. `clickhouse.resources.requests.memory` or nested differently)
  2. What is the minimum ClickHouse memory setting that allows Langfuse to function for
     homelab-scale event volumes (< 1000 events/day)? Verify `clickhouse.settings.max_memory_usage`
     is the correct parameter.
  3. Does the bundled MinIO sub-chart accept a standard `resources:` override, or does it
     use a different value path?
  4. Is `clickhouse.auth.password: changeme` the same static password issue as MinIO's
     `rootPassword: changeme`? If so, document both as needing a secret management
     strategy before production use.

**Definition of done**: Langfuse can be enabled on an 8 GiB node without triggering OOM
evictions on other pods. ClickHouse and MinIO both have explicit memory limits that prevent
unconstrained growth. The fix is verified by checking `kubectl top pods -n langfuse` after
a steady-state deployment.

---

### T4: Add Automated SyncPolicy to Catalog ApplicationSet
**Priority**: Critical
**Related problems**: P3
**Domain**: ArgoCD
**Files/configs to examine**:
  - `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml` — lines 72-83; the
    `syncPolicy` block in the Application template
  - `sikifanso-homelab-bootstrap/bootstrap/root-app.yaml` — lines 37-40; reference
    implementation of `automated.selfHeal: true / prune: true`
  - `sikifanso-homelab-bootstrap/bootstrap/root-agents.yaml` — lines 37-40; same

**Specific questions to answer**:
  1. Is the absence of `automated` in `root-catalog.yaml` intentional? Check git history
     or any inline comment explaining the asymmetry with `root-app` and `root-agents`.
  2. If `automated.selfHeal: true` is added, does the manual `SyncApplication` call in
     `waitForAppear` need to be changed — or can it remain as an accelerator that no longer
     needs to be the sole recovery mechanism?
  3. Does adding `automated.prune: true` to the catalog ApplicationSet create any risk for
     the local-hostPath gitops pattern (e.g., pruning resources that exist in the cluster
     but whose YAML was temporarily absent due to a gitops repo reset)?

**Definition of done**: `root-catalog.yaml` has `automated.selfHeal: true` (and optionally
`prune: true`) in the Application template, consistent with the other two ApplicationSets.
A note documenting the intent is added as a YAML comment.

---

### T5: Fix Hardcoded ~/.kube/config in Install and CreateApplications
**Priority**: High
**Related problems**: P5, CDX-4
**Domain**: K8s / ArgoCD (shared refactor)
**Files/configs to examine**:
  - `internal/argocd/install.go` — line 91; `waitForDeployments` kubeconfig resolution
  - `internal/argocd/applications.go` — line 44; `CreateApplications` kubeconfig resolution
  - `internal/cluster/cluster.go` — lines 148-154; `KubeconfigGetWrite` return value
    handling; where `Install` and `CreateApplications` are called
  - `internal/argocd/appsetreconcile/reconcile.go` — lines 31-38; reference implementation
    that correctly accepts `*rest.Config`

**Specific questions to answer**:
  1. Does `KubeconfigGetWrite` return a `*rest.Config` or does it need to be separately
     obtained? If not, what is the cleanest way to get the `*rest.Config` for the just-created
     cluster — `clientcmd.NewDefaultClientConfigLoadingRules().Load()` with cluster name
     validation, or `k3dclient.KubeconfigGet` + `clientcmd.RESTConfigFromKubeConfig`?
  2. List every call site of `Install` and `CreateApplications` — how many callers need
     to be updated when the signature changes?
  3. Are there any tests that mock `clientcmd.RecommendedHomeFile` — if so, how do they
     need to change?

**Definition of done**: Both `Install(ctx, cfg *rest.Config, ...)` and
`CreateApplications(ctx, cfg *rest.Config, ...)` accept an explicit `*rest.Config`. The
global kubeconfig is not read inside either function. Multi-cluster operation is safe.

---

### T6: Add Missing tier and dependsOn Labels to 14 Catalog Entries
**Priority**: High
**Related problems**: P6, CDX-2
**Domain**: AppWorkloads
**Files/configs to examine**:
  - `sikifanso-homelab-bootstrap/catalog/valkey.yaml` — add `tier: 1-data`
  - `sikifanso-homelab-bootstrap/catalog/loki.yaml` — add `tier: 1-data`
  - `sikifanso-homelab-bootstrap/catalog/tempo.yaml` — add `tier: 1-data`
  - `sikifanso-homelab-bootstrap/catalog/alloy.yaml` — add `tier: 2-services`,
    `dependsOn: [loki, tempo]`
  - `sikifanso-homelab-bootstrap/catalog/litellm-proxy.yaml` — add `tier: 2-services`,
    `dependsOn: [langfuse, ollama]`
  - `sikifanso-homelab-bootstrap/catalog/nemo-guardrails.yaml` — add `tier: 2-services`,
    `dependsOn: [litellm-proxy]`
  - `sikifanso-homelab-bootstrap/catalog/langfuse.yaml` — add `dependsOn: [valkey]`
    (already has `tier: 2-services`)
  - `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml` — confirm RollingSync step
    definitions include `tier: 1-data` as an explicit step

**Specific questions to answer**:
  1. Does `root-catalog.yaml` RollingSync currently have a `tier: 1-data` step, or only
     `0-operators` and `2-services`? If `1-data` is absent, adding `tier: 1-data` to apps
     without a matching ApplicationSet step will have no effect — the step must also be
     declared.
  2. What is the correct `dependsOn` format in the catalog YAML — is it a list of app
     names, ArgoCD Application names, or label selectors? Verify against the ApplicationSet
     generator template.
  3. Should `opa`, `external-secrets`, and `prometheus-stack` be assigned explicit tier
     labels? They are currently in tier-0 but without labels. Confirm whether unlabelled
     apps are treated as tier-0 by default or as an unordered concurrent batch.

**Definition of done**: All 14 apps have `tier` and `dependsOn` fields where applicable.
`root-catalog.yaml` has all three tier steps defined. A cold-start deployment of
`agent-dev` shows valkey healthy before langfuse attempts its Redis connection.

---

### T7: Add valkey to agent-minimal Profile and langfuse dependsOn
**Priority**: High
**Related problems**: P7
**Domain**: AppWorkloads
**Files/configs to examine**:
  - `sikifanso/internal/profile/profile.go` — lines 22-25; agent-minimal app list
  - `sikifanso-homelab-bootstrap/catalog/langfuse.yaml` — lines 10-12; `dependsOn` list
  - `sikifanso-homelab-bootstrap/catalog/values/langfuse.yaml` — lines 29-34; confirm
    `redis.deploy: false` and `redis.host: valkey.storage.svc.cluster.local`

**Specific questions to answer**:
  1. Is `valkey` already enabled in the gitops repo's catalog when `agent-minimal` is
     applied, or does the profile need to enable it? Distinguish between "valkey is
     `enabled: true` in the catalog baseline" vs "valkey must be explicitly added to the
     profile app list to be enabled."
  2. Does the `agent-minimal` profile total memory request remain within the safe 8 GiB
     envelope after adding valkey (valkey requests only 64 MiB)?
  3. Are there any other apps in `agent-minimal` that have undeclared runtime service
     dependencies besides langfuse → valkey?

**Definition of done**: `cluster create --profile agent-minimal` results in langfuse
successfully connecting to valkey on startup. `catalog/langfuse.yaml` lists `valkey` in
`dependsOn`. No other silent missing dependency in the agent-minimal profile.

---

### T8: Fix presidio Analyzer Memory Limit to Accommodate spaCy en_core_web_lg
**Priority**: High
**Related problems**: P9
**Domain**: AppWorkloads / Homelab
**Files/configs to examine**:
  - `sikifanso-homelab-bootstrap/catalog/values/presidio.yaml` — analyzer resources block;
    confirm current `limits.memory: 512Mi`
  - Presidio Helm chart (`presidio/presidio`, chart version in the catalog entry) —
    confirm value path for per-component resource overrides; check whether a smaller
    spaCy model (`en_core_web_sm`) can be configured via a Helm value

**Specific questions to answer**:
  1. Does the `presidio/presidio` Helm chart support a `--spacy-model` or equivalent
     Helm value to select `en_core_web_sm` instead of `en_core_web_lg`? If so, document
     the value path and the memory reduction.
  2. What is the minimum memory limit that allows the analyzer to start without OOM kill
     when using `en_core_web_lg`? Target: 1 GiB limit based on 700-800 MiB observed RSS.
  3. Does the total memory request for `agent-safe` remain within the safe envelope after
     raising the analyzer limit?

**Definition of done**: Presidio analyzer starts and passes its readiness probe on first
deployment without OOM kill. Either the memory limit is raised to 1 GiB or the spaCy model
is switched to `en_core_web_sm` with the smaller limit justified by evidence.

---

### T9: Replace pollOnce Stream Fallback with pollWithRetry Loop
**Priority**: Medium
**Related problems**: P10
**Domain**: Go (ArgoCD orchestrator)
**Files/configs to examine**:
  - `internal/argocd/grpcsync/orchestrator.go` — `pollOnce` call at lines 167-170;
    `pollOnce` implementation at lines 320-336; `pollUntilGone` at approximately lines
    252-278 (reference implementation)
  - `internal/argocd/grpcclient/applications.go` — `WatchApplication` goroutine, lines
    125-148; the `stream.Recv()` error path that closes the channel

**Specific questions to answer**:
  1. When `WatchApplication`'s `eventCh` closes with `!ok`, what does the calling
     `watchSingleApp` know about why the channel closed — is there any way to distinguish
     a clean stream end from a network error at this call site?
  2. Should the retry loop log a warning to the user that the watch stream dropped and
     polling has taken over? What log level and message is appropriate?
  3. Is there a risk that the retry loop produces a final `Synced+Healthy` result from a
     stale poll after the app has already been deleted (race between poll and delete)?

**Definition of done**: When the gRPC watch stream closes unexpectedly mid-sync,
`watchSingleApp` enters a polling loop (retrying at `DefaultPollInterval`) until the app
reaches a terminal state or `ctx` expires. A log warning informs the user that polling
mode is active.

---

### T10: Implement Tier-Aware watchApps Goroutine Sequencing
**Priority**: Medium
**Related problems**: P6, CDX-2
**Domain**: Go (ArgoCD orchestrator)
**Files/configs to examine**:
  - `internal/argocd/grpcsync/orchestrator.go` — `watchApps` function, lines 82-149;
    goroutine launch loop at lines 96-115
  - `internal/argocd/grpcsync/types.go` — `Request` struct; `AppRequest` (per-app
    sub-request if it exists); confirm whether `Tier` is already in the request model
  - `internal/catalog/catalog.go` — `Entry` struct, line 25; confirm `Tier` field and
    its type

**Specific questions to answer**:
  1. Is the `Tier` field from `catalog.Entry` currently threaded into the `grpcsync.Request`
     at any call site? If not, which call sites need to be updated to pass it?
  2. What should `watchApps` do if an app has no `Tier` field — treat it as tier-0, tier-2,
     or a special "unordered" bucket?
  3. Can the tier-sequencing logic reuse the existing `watchSingleApp` function unchanged,
     or does it require changes to how results are propagated back to the caller?
  4. How should the timeout interact with tier sequencing — should each tier get a fraction
     of the total timeout, or should the timeout be per-tier-batch?

**Definition of done**: `watchApps` sequences goroutine launches by tier: all tier-0
apps are watched first; tier-1 goroutines start only after all tier-0 apps reach
`Synced+Healthy`; tier-2 starts after tier-1 completes. Apps with no tier are placed in
the tier-0 bucket by default. Existing tests pass.

---

### T11: Add ArgoCD gRPC Readiness Probe After waitForDeployments
**Priority**: Medium
**Related problems**: P4
**Domain**: K8s / ArgoCD
**Files/configs to examine**:
  - `internal/argocd/install.go` — `waitForDeployments` function, lines 70-133; the
    return point at line 70-73
  - `internal/cluster/cluster.go` — lines 172-192; gap between `argocd.Install` and
    `argocd.CreateApplications`
  - `internal/argocd/grpcclient/client.go` — available methods; check whether a
    `GetVersion` or `ServerVersion` gRPC call is available for use as a readiness ping

**Specific questions to answer**:
  1. Does the ArgoCD gRPC client expose a no-side-effect health or version endpoint that
     can be used as a readiness ping? (e.g., `argocd.io/v1alpha1` `version` endpoint or
     equivalent gRPC method.)
  2. What is the appropriate retry interval and maximum duration for the readiness probe?
     (Suggested: 2s interval, 30s max, based on observed ArgoCD startup time on k3d.)
  3. Does adding the gRPC probe cause issues if ArgoCD TLS is not yet configured — does
     the probe need to handle `insecure` mode consistent with the rest of the client?

**Definition of done**: `cluster create` no longer fails intermittently with
`connection refused` on `CreateApplications`. The gRPC probe retries with exponential
backoff and logs progress. The maximum probe duration is configurable.

---

### T12: Fix Startup ApplicationSet Sequencing in cluster create
**Priority**: Medium
**Related problems**: P17
**Domain**: K8s
**Files/configs to examine**:
  - `internal/cluster/cluster.go` — `gitops.ApplyRootApp` call at lines 195-197; the
    ordering of Cilium, ArgoCD Application syncs, and the root ApplicationSet apply
  - `internal/argocd/grpcsync/orchestrator.go` — whether a `WaitForApps(names []string)`
    primitive exists or needs to be added

**Specific questions to answer**:
  1. After `ApplyRootApp`, is there any wait for Cilium's Application to reach
     `Synced+Healthy` before proceeding? If not, what is the correct point in `cluster.go`
     to insert this gate?
  2. Is the Cilium Application name stable and known at compile time, or is it derived
     from the gitops repo content?
  3. Can the existing `watchApps` be reused to implement a
     `WaitForApps(ctx, names)` that blocks until named apps are healthy — without
     triggering any new syncs?

**Definition of done**: `cluster create` on a constrained (8 GiB RAM) host shows Cilium
reaching `Synced+Healthy` before catalog entries begin syncing. OOM pressure during
bootstrap is measurably reduced.

---

### T13: Build Actionable Error Messages from grpcsync.Result Slice
**Priority**: Medium
**Related problems**: P13
**Domain**: Go
**Files/configs to examine**:
  - `cmd/sikifanso/middleware.go` — `syncAfterMutation`, lines 136-155; static error
    string construction; `printSyncResults` at lines 156-183
  - `internal/argocd/grpcsync/types.go` — `Result` struct, lines 39-46; available fields

**Specific questions to answer**:
  1. Should the per-app failure details be embedded in the returned `error` value, or in a
     separate structured log line? What format works best for both interactive terminal and
     CI JSON log output?
  2. For `ExitTimeout`, the app may still be reconciling — should the error message
     distinguish "apps that were `Progressing` at timeout" from "apps that were
     `Unknown`/`Degraded` at timeout"?
  3. Is there a character limit or line-count concern for embedding multiple app statuses
     in a single error string for terminal display?

**Definition of done**: When `app enable` fails, the error message includes the name of
the failing app(s), their `SyncStatus`, `Health`, and at least the first degraded resource
message. CI log output retains this information without requiring a separate stderr block.

---

### T14: Fix Spinner Data Race and Multi-App Progress Display
**Priority**: Medium
**Related problems**: P15
**Domain**: Go
**Files/configs to examine**:
  - `cmd/sikifanso/middleware.go` — `syncAfterMutation`, lines 100-125; spinner
    construction at line 103; `OnProgress` closure at lines 114-120
  - `internal/argocd/grpcsync/orchestrator.go` — goroutine launch in `watchApps`,
    lines 96-115; `req.OnProgress` call sites

**Specific questions to answer**:
  1. Does the spinner library used (`github.com/briandowns/spinner` or similar) expose a
     thread-safe `Suffix` setter? If so, can it be used as a drop-in fix without changing
     the rendering model?
  2. What is the simplest multi-app progress display that fits the existing CLI UX? Options:
     (a) single-line spinner showing "3/5 apps synced", (b) multi-line status with one line
     per app, (c) channel-based renderer. Which minimises changes to existing `OnProgress`
     contract?
  3. Confirm that `go test -race ./cmd/sikifanso/...` currently surfaces this race and
     that the fix eliminates it.

**Definition of done**: `go test -race ./cmd/sikifanso/...` passes cleanly (no race
detected). Users watching `app enable` with 3+ apps see progress for all apps, not just
the last one to update.

---

### T15: Constrain Temporal to Four Per-Service Resource Blocks and Disable Elasticsearch
**Priority**: Medium
**Related problems**: P21
**Domain**: Homelab / AppWorkloads
**Files/configs to examine**:
  - `sikifanso-homelab-bootstrap/catalog/values/temporal.yaml` — full file; current
    `server.resources` block; confirm which Helm values control per-service resources
  - Temporal Helm chart source (`go.temporal.io/helm-charts`, version matching catalog
    entry) — `values.yaml` for `frontend`, `history`, `matching`, `worker` top-level keys;
    `elasticsearch` sub-chart default enabled state

**Specific questions to answer**:
  1. Does `server.resources` in the temporal chart apply to all four services or only to
     a wrapper? Check chart `_helpers.tpl` or equivalent to confirm which template actually
     uses this value.
  2. What is the minimum per-service resource configuration for a single-node homelab
     with light temporal workloads (1-2 workflows in flight)?
  3. Is `elasticsearch.enabled: false` sufficient to prevent schema migration jobs from
     running, or is there an additional `schema.update.enabled: false` needed?

**Definition of done**: Temporal values file has explicit `frontend.resources`,
`history.resources`, `matching.resources`, and `worker.resources` blocks. Elasticsearch
sub-chart is explicitly disabled. `kubectl describe pods -n temporal` shows all four
service pods respecting their configured limits.

---

### T16: Reduce Prometheus PVC to 5 GiB and Add retentionSize Guard
**Priority**: Medium
**Related problems**: P22
**Domain**: Homelab
**Files/configs to examine**:
  - `sikifanso-homelab-bootstrap/catalog/values/prometheus-stack.yaml` — `storage: 20Gi`
    line 23-25; `retention: 7d` lines 18-20; `prometheusSpec` section

**Specific questions to answer**:
  1. What is the correct Helm value path for `retentionSize` under the
     `kube-prometheus-stack` chart — is it `prometheusSpec.retentionSize` or a different
     nesting?
  2. Should AlertManager's 5 GiB PVC also be reduced? What is the typical AlertManager
     state storage requirement for a homelab cluster with no long-term silence/inhibition
     state?
  3. After reducing Prometheus to 5 GiB and AlertManager to 1 GiB, does total PVC
     allocation drop to a manageable level for an `agent-dev` profile?

**Definition of done**: Prometheus PVC is 5 GiB with `retentionSize: "4GB"` configured.
AlertManager PVC is 1 GiB. Total PVC allocation for `agent-dev` profile is documented.

---

### T17: Separate ResourceQuota Requests and Limits in agent-template
**Priority**: Medium
**Related problems**: P23
**Domain**: Homelab / AppWorkloads
**Files/configs to examine**:
  - `sikifanso-agent-template/templates/resourcequota.yaml` — lines 9-14; current quota
    field mapping
  - `sikifanso-agent-template/values.yaml` — lines 4-8; `cpu` and `memory` single values
  - `internal/agent/` in `sikifanso/` — how `agent create` passes cpu/memory values to
    the Helm chart; what flags are exposed to the user

**Specific questions to answer**:
  1. What is the correct Kubernetes ResourceQuota field semantics for allowing Burstable
     pods — should only `limits.cpu` and `limits.memory` be set (omitting request-side
     quota), or should requests quota be set at a lower value than limits quota?
  2. Does `sikifanso agent create` expose `--cpu` and `--memory` flags? If so, do they
     currently set both request and limit to the same value?
  3. Are there existing agent workloads or tests that would break if the quota fields are
     changed?

**Definition of done**: Agent namespace ResourceQuota allows Burstable QoS (request <
limit). The `agent create` command exposes separate `--cpu-request`/`--cpu-limit` flags or
documents sensible defaults for orchestration vs inference workloads.

---

## Out of Scope

The following observations were identified during the review but are deliberately deferred:

- **Standalone ClickHouse and MinIO catalog entries** (CDX-3 strategic direction): Creating
  new catalog entries requires design decisions about versioning, chart selection, and
  Langfuse wiring that go beyond the scope of a resource-constraints fix. Deferred to a
  dedicated feature session after T3 (immediate constraints) is complete.

- **Local registry mirror / image pre-pull DaemonSet** (K8s Finding 5): Pre-pulling images
  reduces the probability of EOF-triggered `Degraded` states but is infrastructure
  complexity that should be evaluated after T1 (grace period) eliminates false negatives.
  Deferred.

- **PgBouncer catalog entry** (AppWorkloads Finding 13, max_connections risk): The
  max_connections=200 ceiling is a real concern under agent-full, but requires a new
  catalog entry and connection string changes in langfuse and temporal values. Deferred
  until actual connection exhaustion is observed.

- **WatchApplication goroutine cleanup on ctx cancel** (Go Finding 6): The race is largely
  theoretical because gRPC propagates context cancellation through the stream. The residual
  risk (non-blocking ctx poll pattern) is low-severity polish. Deferred.

- **Ollama memory request vs realistic working set** (AppWorkloads Finding 7 / Homelab
  Finding 6): Raising ollama's request from 1 GiB to 2 GiB is correct but has no
  functional impact on a single-node cluster (scheduler always places on the one node).
  Deferred to a resource-sizing consolidation session.

- **Loki filesystem storage durability** (AppWorkloads Finding 9): Log loss on cluster
  recreate is a documented characteristic of the homelab design, not a bug. Deferred until
  a shared MinIO catalog entry exists to offer an upgrade path.

- **text-embeddings-inference CPU-only throughput** (AppWorkloads Finding 8): CPU-mode TEI
  throughput is a performance concern, not a reliability defect. Deferred.

- **Prometheus scrape interval and TSDB tuning** (Homelab Finding 8): The
  `retentionSize` guard is included in T16; further TSDB tuning (scrape interval, head
  compaction) is deferred.

- **opa / qdrant / prometheus-stack tier label additions** (AppWorkloads Findings 14-16):
  These are informational-only dependency concerns with no runtime boot-order risk for
  current profiles. Deferred as low-priority catalog hygiene.
