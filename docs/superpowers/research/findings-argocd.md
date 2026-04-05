# ArgoCD Integration: Code Review Findings

## Executive Summary

The sikifanso ArgoCD integration has one confirmed production defect: the CLI exits with a failure on `Synced+Degraded` even when that state is a normal transient startup condition (CRD establishment, PVC binding, image pull back-off), causing false-negative results that recover on their own. The catalog ApplicationSet deliberately omits `automated.selfHeal`/`prune`, meaning that if the CLI-triggered sync races with the AppSet controller, apps can silently stall in `OutOfSync` without any re-drive. Several lower-severity issues compound this: a single-shot `pollOnce` fallback offers no resilience after a stream drop, the `waitForAppear` polling starts immediately before the AppSet controller has had time to act, and the `ResourceTree` call used for failure diagnostics pulls node-level data that misses the more useful `app.Status.Resources` sync-error messages already in `GetApplication`.

---

## Finding 1: `Synced+Degraded` Is Treated as Final When It Is Often Transient

**Severity**: Critical

**Domain**: ArgoCD

**Affected scenario**: `app enable` with auto-deps (e.g. `sikifanso app enable postgresql` auto-enables `prometheus-stack`)

**Root cause hypothesis**: ArgoCD's health model does not distinguish "app just synced and resources are still initialising" from "app is permanently broken." CRDs always pass through `Degraded` ("CRD is not established") for several seconds before the API server registers them. PVCs are `Progressing` (not `Degraded`) while binding, but the pod that depends on them may itself become `Degraded` if the image pull fails transiently. The exit condition at `orchestrator.go:195` fires as soon as the first `Synced+Degraded` event arrives from the watch stream, which for a freshly-synced Helm chart can happen within seconds of the sync completing — before any resource has had a chance to reach steady state.

**Evidence**:
- `internal/argocd/grpcsync/orchestrator.go:195` — exits immediately on `Health == "Degraded" && SyncStatus == "Synced" && !req.SkipUnhealthy`, with no minimum observation window or resource-type carve-outs.
- The observed failure log shows `CRD/prometheusagents.monitoring.coreos.com health=Degraded CRD is not established` — this is the canonical transient CRD state; ArgoCD itself marks it `Degraded` for 5–30 s while the API server processes it.
- `internal/argocd/grpcsync/orchestrator.go:183-189` — the `Result` passed to `updateFn` before the early-exit does not record a timestamp or observation count, so there is no way to add a "seen degraded for N seconds" gate without structural changes.
- The comment at line 152-153 acknowledges the OutOfSync case was already fixed, but the transient `Synced+Degraded` case was not.

**Cross-domain flag**: No

**Investigation direction**: Introduce a minimum-observation window (e.g. require `Synced+Degraded` to persist for at least 30 s with no intervening `Healthy` events) before declaring failure. Alternatively, whitelist specific resource kinds (CRD, PVC, Pod while its image is being pulled) by inspecting `r.Resources` and only exiting early when the degraded resources are not in a known-transient set. The ArgoCD canonical approach is to consult `app.Status.OperationState.Phase` — if it is `Running` or `Succeeded` and health is `Degraded`, the app is still converging; only `Failed` + `Degraded` is a hard failure.

---

## Finding 2: Catalog ApplicationSet Has No `automated` SyncPolicy — Silent Stall Window

**Severity**: Critical

**Domain**: ArgoCD

**Affected scenario**: `app enable` / `app disable` for any catalog entry

**Root cause hypothesis**: `root-catalog.yaml` defines no `automated` block in its Application template. This means the Application CR created by the AppSet controller has no auto-sync; the only driver is the manual `SyncApplication` call in `waitForAppear` (`orchestrator.go:241`). If that call arrives while ArgoCD is still processing the previous reconcile cycle, or if the sync request is rejected (e.g. another operation is already in flight), the app will stay `OutOfSync` indefinitely — there is no background self-heal to recover it. The `root-app.yaml` and `root-agents.yaml` AppSets correctly set `automated.selfHeal: true` / `prune: true`, making this an inconsistency rather than an intentional choice.

**Evidence**:
- `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml:72-83` — `syncPolicy` block contains only `syncOptions` and `retry`; no `automated` key.
- `sikifanso-homelab-bootstrap/bootstrap/root-app.yaml:37-40` and `root-agents.yaml:37-40` — both set `automated.selfHeal: true` / `prune: true`.
- `internal/argocd/grpcsync/orchestrator.go:241` — `SyncApplication` is the sole sync trigger for catalog apps after they appear; log message "sync trigger after appear failed (may already be syncing)" is swallowed at Debug level.

**Cross-domain flag**: No

**Investigation direction**: Add `automated.selfHeal: true` / `prune: true` to the catalog ApplicationSet template, consistent with `root-app` and `root-agents`. The manual `SyncApplication` call in `waitForAppear` can remain as an accelerator to avoid the 3-minute reconcile interval, but it should no longer be the sole mechanism. If the absence of `automated` is intentional (e.g. to prevent background churn in a dev cluster), document it and add a retry loop around the `SyncApplication` call with exponential back-off instead of a single fire-and-forget.

---

## Finding 3: `pollOnce` as Stream Fallback Is Insufficient

**Severity**: High

**Domain**: ArgoCD

**Affected scenario**: Any `watchSingleApp` call where the gRPC stream drops mid-watch

**Root cause hypothesis**: When the gRPC watch stream closes unexpectedly (`eventCh` channel is closed without a `Deleted` event), `watchSingleApp` calls `pollOnce` exactly once and then returns. A single `GetApplication` snapshot may catch the app in any transient intermediate state (e.g. `OutOfSync/Progressing` mid-sync) and record it as the final result, causing a spurious timeout or false healthy/unhealthy report. In a local k3d cluster, the gRPC stream is particularly fragile: there is no load balancer keep-alive, the ArgoCD server pod may restart during cluster bootstrap, and the stream has no client-side heartbeat timeout configured (no `keepalive` or `WaitForReady` options visible in `grpcclient/client.go`).

**Evidence**:
- `internal/argocd/grpcsync/orchestrator.go:167-170` — `case event, ok := <-eventCh:` with `if !ok { o.pollOnce(...); return }`.
- `internal/argocd/grpcclient/applications.go:134-136` — `stream.Recv()` error causes the goroutine to return and close the channel; any `recvErr` — including transient network errors — is silent and indistinguishable from a clean stream close.
- `internal/argocd/grpcclient/client.go:44-85` — `apiclient.ClientOptions` sets no gRPC `DialOptions`, `keepalive.ClientParameters`, or `WaitForReady`.
- `waitForDisappear` at `orchestrator.go:274-278` also falls back to `pollUntilGone` (which polls in a loop), so it is more resilient than `watchSingleApp`'s single-shot fallback — the asymmetry is inconsistent.

**Cross-domain flag**: No

**Investigation direction**: Replace `pollOnce` with a `pollUntilTerminal` loop (similar to `pollUntilGone`) that keeps polling at `DefaultPollInterval` until the app reaches `Synced+Healthy`, `Degraded`, or ctx timeout. Additionally, consider reopening the watch stream after a transient close (exponential back-off, up to the context deadline) before falling back to polling.

---

## Finding 4: `waitForAppear` First Poll Fires After 5 s, But AppSet Controller May Need Longer

**Severity**: High

**Domain**: ArgoCD

**Affected scenario**: `app enable` — any catalog or custom app

**Root cause hypothesis**: `waitForAppear` starts its ticker immediately and polls every 5 s. However, after `Trigger()` patches the annotation, the AppSet controller must: (1) detect the annotation, (2) re-evaluate the git generator (reads the local hostPath), (3) render the template, (4) create the Application CR. Under normal load this is fast (< 5 s), but during cluster bootstrap when the AppSet controller pod itself may be restarting or under high reconcile queue pressure, the Application CR can take 10–30 s to appear. More critically, `Trigger()` returns immediately after the Kubernetes PATCH call completes — there is no confirmation that the controller has acted on it. The annotation is removed by the controller after reconciliation, but `waitForAppear` never checks whether the annotation was removed as a signal that reconciliation ran.

**Evidence**:
- `internal/argocd/appsetreconcile/reconcile.go:45-56` — `Trigger` patches annotation and returns; no watch/poll to confirm annotation removal.
- `internal/argocd/grpcsync/orchestrator.go:214-247` — `waitForAppear` loops on `GetApplication` returning `NotFound`; if the Application exists from a previous run it will pass through immediately and call `SyncApplication` before the AppSet controller has re-rendered it with updated values.
- `internal/argocd/grpcsync/orchestrator.go:43-46` — `ReconcileFn` (annotation patch) error is only logged as `Warn` and execution continues to `watchApps`; this means `waitForAppear` will block for the full timeout if the patch itself failed silently.

**Cross-domain flag**: No

**Investigation direction**: After `Trigger()`, poll the ApplicationSet CR to wait for the annotation to be removed (confirming the controller acted on it) before starting `waitForAppear`. Alternatively, watch the ApplicationSet CR with a field selector and use `resourceVersion` to confirm a new generation was processed. The annotation removal is documented ArgoCD behaviour for `argocd.argoproj.io/application-set-refresh`.

---

## Finding 5: `SyncApplication` Called After `waitForAppear` With No Operation-State Guard

**Severity**: High

**Domain**: ArgoCD

**Affected scenario**: `app enable` where AppSet has `automated.selfHeal: true` (root, agents AppSets)

**Root cause hypothesis**: `waitForAppear` calls `SyncApplication` as soon as `GetApplication` returns a non-NotFound result (`orchestrator.go:241`). For `root` and `agents` ApplicationSets, the Application is created with `automated.selfHeal: true` / `prune: true`, meaning ArgoCD's controller may have already started an automatic sync. Sending a manual `SyncApplication` while ArgoCD's internal operation is `Running` causes ArgoCD to return a gRPC error ("another operation is already in progress"), which is currently swallowed at Debug level. If the auto-sync finishes before the watch stream is established, the initial state event may arrive showing `Synced+Healthy`, which is correct — but if it arrives showing `OutOfSync/Progressing` (the sync is running), the watcher will keep waiting. The real risk is the opposite: the manual sync request may interrupt a carefully-ordered sync (e.g. CRD before workload), causing resource-ordering failures.

**Evidence**:
- `internal/argocd/grpcsync/orchestrator.go:241-243` — `SyncApplication` is called unconditionally; the error is `Debug`-logged with message "may already be syncing".
- `internal/argocd/grpcclient/applications.go:92-107` — `SyncApplication` wraps `client.Sync`; no check for in-flight operations.
- `sikifanso-homelab-bootstrap/bootstrap/root-app.yaml:37-40` and `root-agents.yaml:37-40` — `automated.selfHeal: true / prune: true`; these apps will auto-sync; the manual trigger is redundant and potentially disruptive.
- `internal/argocd/grpcclient/applications.go:98-100` — `SyncOptions` only exposes `Prune`; no `DryRun`, `Force`, or `RetryStrategy` to handle the "already syncing" case.

**Cross-domain flag**: No

**Investigation direction**: Before calling `SyncApplication`, call `GetApplication` and inspect `app.Status.OperationState.Phase`. If it is `Running`, skip the manual trigger and proceed directly to `watchSingleApp`. For the `catalog` AppSet (no `automated`), the manual sync is necessary; for `root` and `agents`, it should be conditional.

---

## Finding 6: ResourceTree Used for Failure Diagnostics Misses Sync Error Messages

**Severity**: Medium

**Domain**: ArgoCD

**Affected scenario**: Any failed sync — error detail shown under the `✗` app in terminal output

**Root cause hypothesis**: When `watchSingleApp` detects `Synced+Degraded`, it calls `ResourceTree` (`orchestrator.go:197`) to populate `r.Resources` for display. `ResourceTree` returns `tree.Nodes` (the full resource graph from the ArgoCD cache), which has node-level health and message fields. However, `app.Status.Resources` (returned by `GetApplication` and already mapped in `toResourceStatuses`) contains the per-resource `SyncStatus` alongside the health, which is more actionable for diagnosing sync failures (e.g. "hook failed", "resource already exists"). Additionally, `ResourceTree.Nodes` do not include the `SyncStatus` field at all — only health. The displayed error messages under failing resources may therefore be incomplete or misleading.

**Evidence**:
- `internal/argocd/grpcclient/applications.go:154-181` — `ResourceTree` maps `tree.Nodes` to `ResourceStatus`; `node.Health` is present but `node` has no `Status` (sync status) field.
- `internal/argocd/grpcclient/applications.go:71-89` — `toResourceStatuses` maps `app.Status.Resources` which includes both `r.Status` (sync status) and `r.Health`.
- `internal/argocd/grpcclient/types.go:40-48` — `ResourceStatus` has both `Status` (sync) and `Health` fields, but the `ResourceTree` path leaves `Status` empty.
- `cmd/sikifanso/middleware.go:172-181` — `printSyncResults` prints `res.Health` and `res.Message` but not `res.Status`; unhealthy-but-synced resources would be silent.

**Cross-domain flag**: No

**Investigation direction**: Replace the `ResourceTree` call in the `Degraded` exit path with an additional `GetApplication` call (which is already available via `o.client.GetApplication`) and use `detail.Resources` (which has both sync status and health). The `ResourceTree` call is more expensive (fetches the full graph) and less informative for the common sync-failure diagnostic case. Only fall back to `ResourceTree` for live-state diffs.

---

## Finding 7: All Apps Watched Concurrently, Ignoring Catalog `RollingSync` Tier Ordering

**Severity**: Medium

**Domain**: ArgoCD

**Affected scenario**: `app enable` with multiple apps across different tiers (e.g. tier-0 operator + tier-2 service)

**Root cause hypothesis**: `watchApps` (`orchestrator.go:82`) launches a goroutine per app concurrently with `sync.WaitGroup`. The catalog ApplicationSet is configured with `strategy.type: RollingSync` with three tiers (`0-operators`, `1-data`, `2-services`). ArgoCD's RollingSync will not start syncing tier-1 until all tier-0 apps are `Healthy`. But the CLI watches all apps concurrently and will report tier-1/2 apps as timing out (still `OutOfSync/Unknown`) even though they are intentionally held back by ArgoCD. This produces confusing output and potentially an incorrect `ExitTimeout` code when the tier sequencing simply hasn't advanced yet.

**Evidence**:
- `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml:7-25` — `strategy.type: RollingSync` with three steps by `tier` label.
- `internal/argocd/grpcsync/orchestrator.go:96-115` — all apps launched concurrently; no tier-awareness.
- `internal/argocd/grpcsync/orchestrator.go:127-145` — `ExitTimeout` is the code when `SyncStatus != "Synced" || Health != "Healthy"`; a tier-2 app held at `OutOfSync/Unknown` by RollingSync would produce this code even though ArgoCD is doing the right thing.
- `internal/catalog/catalog.go:25` — `Entry` has a `Tier` field, so the data is available to the CLI.

**Cross-domain flag**: No

**Investigation direction**: The simplest fix is to not treat `OutOfSync/Unknown` (or `OutOfSync/Missing`) as a hard timeout if the app has a tier label and a lower-tier app is still `Progressing`. A more complete fix is to sequence the `waitForAppear` goroutines by tier: start watching tier-0 first, wait for all tier-0 to be `Synced+Healthy`, then start watching tier-1, and so on. The `catalog.Entry.Tier` field is already populated; it just needs to be threaded into the `Request` type.

---

## Finding 8: gRPC Connection Has No Keep-Alive or Retry Policy

**Severity**: Medium

**Domain**: ArgoCD

**Affected scenario**: Long-running watches (e.g. slow Helm chart with many resources, or cluster under load)

**Root cause hypothesis**: The gRPC client is created with plain `apiclient.ClientOptions` and no additional `DialOptions`. There are no `keepalive.ClientParameters` configured, meaning that idle streams (no events for > 20 s) may be silently dropped by intermediate network components (Docker's internal bridge, CNI, or the OS TCP stack). In a k3d cluster running inside Docker, the bridge network has a connection-tracking timeout that can silently drop idle TCP connections. When this happens, `stream.Recv()` will block until the next event or context cancellation — it will not return an error immediately. This means the fallback to `pollOnce` may not be triggered even when the stream is dead.

**Evidence**:
- `internal/argocd/grpcclient/client.go:44-85` — no `grpc.WithKeepaliveParams`, no `grpc.WithBlock`, no `grpc.WithReturnConnectionError`.
- `internal/argocd/grpcclient/applications.go:125-148` — the goroutine calls `stream.Recv()` in a tight loop with only a `ctx.Done()` select guard; a silently-dead stream will block `stream.Recv()` indefinitely without firing the `!ok` branch.
- The `WatchApplication` channel has a buffer of 16 (`make(chan WatchEvent, 16)`); buffer exhaustion is not detected.

**Cross-domain flag**: No

**Investigation direction**: Add `grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 20*time.Second, Timeout: 5*time.Second, PermitWithoutStream: true})` to the dial options. Add a separate `time.AfterFunc` deadline inside the watch goroutine: if no event is received within `N` seconds, close the channel explicitly to force the fallback path. This is a known pattern for gRPC watch loops.

---

## Finding 9: `applications.go` Uses `~/.kube/config` Not the Session's Kubeconfig

**Severity**: Low

**Domain**: ArgoCD

**Affected scenario**: Multi-cluster setups where the active kubeconfig context differs from the target cluster

**Root cause hypothesis**: `CreateApplications` in `internal/argocd/applications.go:44` calls `clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)` which reads `~/.kube/config` using the current active context. Every other Kubernetes operation in sikifanso (AppSet reconciler, kube package) uses the cluster-specific kubeconfig from the session. If the user's active context points to a different cluster (e.g. a production cluster or a different k3d cluster), `CreateApplications` will silently create the Application CRDs in the wrong cluster.

**Evidence**:
- `internal/argocd/applications.go:44-47` — `clientcmd.RecommendedHomeFile` is hardcoded; no cluster name or session path is passed.
- `internal/argocd/appsetreconcile/reconcile.go:31-38` — correctly accepts `*rest.Config` as a parameter, allowing the caller to provide the right cluster's config.
- `internal/kube/` — presumably provides `RESTConfigForCluster` (referenced in `middleware.go:95`); `applications.go` does not use it.

**Cross-domain flag**: No

**Investigation direction**: Change `CreateApplications` signature to accept a `*rest.Config` parameter (like `appsetreconcile.NewReconciler` does) and remove the internal `clientcmd.BuildConfigFromFlags` call. All call sites should pass the REST config obtained from `kube.RESTConfigForCluster(sess.ClusterName)`.
