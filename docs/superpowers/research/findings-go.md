# Go Code Review Findings — sikifanso CLI

## Executive Summary

The observed failure (`app enable postgresql` → `sync failed: one or more apps unhealthy`) is reproducible and has a clear root cause: `watchSingleApp` exits on `Synced+Degraded` and classifies that terminal state as `ExitFailure`, but Kubernetes workloads routinely pass through a `Synced+Degraded` window as containers pull images and probes warm up. Five additional findings compound the UX damage — the spinner is racy, error messages discard all actionable context, the post-profile sync in `cluster create` uses `OpSync` (no AppSet reconciliation) against a stale app list, `waitForAppear` has a first-tick latency gap, and the `WatchApplication` goroutine can block `closer.Close()` for the full gRPC stream idle timeout if the context cancels while `Recv` is in flight.

---

## Finding 1: False-Positive Failure on Slow-Starting Apps

**Severity**: Critical  
**Domain**: Go  
**Affected scenario**: `app enable` with auto-deps (e.g. `postgresql` pulls in `prometheus-stack`)  
**Root cause hypothesis**: `watchSingleApp` exits immediately when it sees `SyncStatus == "Synced" && Health == "Degraded"` and hands off that state to `watchApps`, which classifies it as `ExitFailure`. Kubernetes workloads legitimately pass through `Synced+Degraded` while pods are scheduling, pulling images, or failing liveness probes for the first time. ArgoCD reflects this as `Degraded` before pods stabilise. The fix comment in the source (`// FIX: Don't exit on Degraded immediately`) acknowledges this but the implementation still exits unconditionally.  
**Evidence**:  
- `grpcsync/orchestrator.go:195–205` — the `Synced+Degraded` branch fetches the resource tree and calls `return`, closing the watch immediately.  
- `grpcsync/orchestrator.go:134–138` — `watchApps` exit-code logic maps `Health == "Degraded"` to `ExitFailure` with no grace period.  
- `middleware.go:145` — `ExitFailure` produces `"sync failed: one or more apps unhealthy"` with no retry or advisory.  
**Cross-domain flag**: no  
**Investigation direction**: Introduce a configurable `DegradedGracePeriod` (e.g. 60 s default). When the app first enters `Synced+Degraded`, start a timer; if health recovers before the timer fires, continue watching. Only emit `ExitFailure` if the grace period expires while still Degraded. Alternatively, re-open the watch instead of returning — but the grace-period approach is safer because it bounds wait time.

---

## Finding 2: cluster create Post-Profile Sync Uses Wrong Operation and Stale App List

**Severity**: High  
**Domain**: Go  
**Affected scenario**: `cluster create --profile agent-dev` (or any profile)  
**Root cause hypothesis**: After `profile.Apply` commits catalog YAML, `cluster_create.go` calls `client.ListApplications` and passes the result straight into `orch.SyncAndWait` with `OpSync` (the zero-value default). `OpSync` calls `SyncApplication` on each app; it does not trigger AppSet reconciliation. The newly-enabled catalog apps exist only as YAML files in the gitops repo at this point — the ArgoCD ApplicationSet controller has not yet reconciled them into Application CRs. `ListApplications` will therefore either return an empty list (no apps created yet) or return only the root ApplicationSets themselves, missing every newly-enabled catalog app. When the list is empty the sync is silently skipped (`if len(names) > 0`). When it is non-empty but stale the sync targets wrong or already-healthy apps and still exits cleanly, giving a false impression that the profile was deployed.  
**Evidence**:  
- `cluster_create.go:115–129` — `ListApplications` → `names` → `SyncAndWait` with no `Operation` field set (defaults to `OpSync`), no `ReconcileFn`, no `OnProgress`.  
- `grpcsync/types.go:19` — `OpSync OperationType = iota` (value 0, the zero value).  
- `grpcsync/orchestrator.go:57–64` — `OpSync` path calls `SyncApplication` directly; no AppSet reconcile.  
- `profile/profile.go:110–176` — `Apply` commits files and returns; it never triggers ArgoCD.  
**Cross-domain flag**: no  
**Investigation direction**: Replace the `ListApplications`+`OpSync` pattern with `OpEnable` using the profile app names directly and supply a `ReconcileFn` that triggers the `catalog` ApplicationSet (identical to what `syncAfterMutation` does in `middleware.go:111–121`). This mirrors the path already used by `app enable`.

---

## Finding 3: Spinner Suffix Is a Data Race Under Concurrent Watches

**Severity**: High  
**Domain**: Go  
**Affected scenario**: `app enable` with multiple apps syncing concurrently  
**Root cause hypothesis**: `watchApps` spawns one goroutine per app (`orchestrator.go:96–115`). Each goroutine calls `req.OnProgress`, which directly assigns `s.Suffix` (`middleware.go:115–119`). The `spinner` library does not protect `Suffix` with a mutex in its public API — the background spinner goroutine reads `s.Suffix` on every tick while the watch goroutines write it. This is a data race. Additionally, the suffix is overwritten by whichever goroutine fires last; earlier progress from other apps is lost.  
**Evidence**:  
- `middleware.go:103` — `s := spinner.New(...)` (single spinner, single `Suffix` field).  
- `middleware.go:114–120` — `OnProgress` closure writes `s.Suffix` directly.  
- `orchestrator.go:96–115` — multiple goroutines all invoke `req.OnProgress` concurrently.  
- `go test -race` on any `app enable` with ≥2 apps will surface this as a race on `s.Suffix`.  
**Cross-domain flag**: no  
**Investigation direction**: (a) Replace the single spinner with a multi-line progress renderer that maintains a `map[string]string` of per-app status, protected by a `sync.Mutex`, and re-renders all lines on each `OnProgress` call. Or (b) replace `OnProgress` with a buffered status channel read by a single renderer goroutine, eliminating the mutex entirely. The `charmbracelet/lipgloss` or a simple ANSI-escape approach works well here.

---

## Finding 4: Error Messages Discard All Available Context

**Severity**: High  
**Domain**: Go  
**Affected scenario**: Any sync failure or timeout  
**Root cause hypothesis**: `syncAfterMutation` receives the full `[]grpcsync.Result` slice including per-resource health details, but the error messages it returns to the user contain only static strings. The user is told `"sync failed: one or more apps unhealthy"` or `"sync timed out: apps may still be reconciling"` with no indication of which app failed, what its health/sync state was, or which resources are degraded. `printSyncResults` writes this detail to stderr before the error is returned, but if the terminal clears (e.g. in CI) or the user piped output, none of it survives.  
**Evidence**:  
- `middleware.go:136–151` — error construction uses only static strings; `results` is ignored.  
- `middleware.go:156–183` — `printSyncResults` has the data but it is called before the error (line 138), which is fine in interactive mode but lossy in CI.  
- `grpcsync/types.go:39–46` — `Result` carries `App`, `SyncStatus`, `Health`, `Message`, `Resources` — all discarded by the error message.  
**Cross-domain flag**: no  
**Investigation direction**: Build the error string from the result slice: iterate results, collect those with non-success health, and append `app=NAME sync=X health=Y` pairs. For timeout, list which apps had not yet reached `Synced+Healthy`. This gives the user — and CI logs — enough context to act without reading a separate stderr block.

---

## Finding 5: waitForAppear First-Poll Delay Is Always DefaultPollInterval

**Severity**: Medium  
**Domain**: Go  
**Affected scenario**: `app enable` with `OpEnable`, fast-reconciling ApplicationSets  
**Root cause hypothesis**: `waitForAppear` creates a ticker and then immediately enters a `select` that waits for either `ctx.Done` or `ticker.C` (`orchestrator.go:219–237`). The tick fires after `DefaultPollInterval` (5 seconds), meaning the function always waits at least 5 seconds before checking whether the app exists — even when the ApplicationSet controller has already reconciled and the Application CR appeared in < 1 second. For fast clusters this adds unnecessary latency to every `app enable`.  
**Evidence**:  
- `grpcsync/types.go:12` — `DefaultPollInterval = 5 * time.Second`.  
- `orchestrator.go:215–235` — ticker created, select blocks immediately on `ticker.C`; no initial check before the first tick.  
**Cross-domain flag**: no  
**Investigation direction**: Add an immediate `GetApplication` call before entering the poll loop — the same pattern used by `waitForDisappear` at lines 252–259. If the app already exists, skip straight to `watchSingleApp`. This turns the common fast path from ≥5 s to ≤1 s.

---

## Finding 6: WatchApplication Goroutine Can Block on Recv After Context Cancel

**Severity**: Medium  
**Domain**: Go  
**Affected scenario**: Timeout or user interrupt (Ctrl-C) during any watch  
**Root cause hypothesis**: The goroutine in `WatchApplication` uses a non-blocking ctx check (`select { case <-ctx.Done(): return; default: }`) before calling `stream.Recv()` (`applications.go:129–135`). `stream.Recv()` is a blocking call. If the context cancels after the `default` branch is taken, the goroutine blocks inside `Recv` until the gRPC server closes the stream — which for a long-running watch may be many seconds or until the idle timeout fires. During this window `closer.Close()` (the gRPC sub-connection) is not called, and the goroutine leaks until `Recv` unblocks. The outer `ctx` cancellation should cause `Recv` to return with an error because the context is attached to the gRPC stream, but this relies on gRPC propagating the cancellation through `stream.Context()`, which only works if the stream was opened with the same context (it is, at line 118). So in practice the race window is small — but the pattern is still fragile.  
**Evidence**:  
- `applications.go:129–147` — non-blocking ctx poll before `Recv`; no `select` on `Recv` result.  
- `applications.go:125–148` — goroutine defers `closer.Close()`; it does not close until `Recv` returns.  
- `client.go:108` — `Client.Close()` is a no-op, so there is no outer close racing with the goroutine's `closer.Close()` (the race question from the prompt is therefore not an issue as currently coded, but only because `Close()` is a no-op).  
**Cross-domain flag**: no  
**Investigation direction**: The current implementation is mostly safe because gRPC does propagate context cancellation through the stream. The main residual risk is the non-blocking ctx poll pattern; replace the `select { default }` pattern with a pure context-driven approach: rely on `stream.Recv()` returning a non-nil error when the context is cancelled, and remove the pre-check loop entirely. Also document that `Client.Close()` is a no-op so callers do not assume it cancels active streams.

---

## Finding 7: ExitCode Default Branch Conflates Progressing and Unknown

**Severity**: Medium  
**Domain**: Go  
**Affected scenario**: Any sync that times out while an app is still `Progressing` or `Unknown`  
**Root cause hypothesis**: The `watchApps` exit-code logic (`orchestrator.go:127–143`) has three cases: explicit success (`Synced+Healthy` or `Deleted`), explicit failure (`Degraded` or `Missing`), and a `default` branch that maps everything else to `ExitTimeout`. The `default` branch silently covers `Progressing`, `Unknown`, `Suspended`, and any future ArgoCD health state strings. An app that is `Unknown` because ArgoCD lost contact with the cluster is indistinguishable from an app that is legitimately `Progressing`. Both produce `ExitTimeout` and the same error message. `Unknown` typically warrants a louder failure signal.  
**Evidence**:  
- `orchestrator.go:134–143` — `default` case maps to `ExitTimeout`.  
- `middleware.go:149–150` — both `ExitTimeout` cases produce `"sync timed out: apps may still be reconciling"`.  
**Cross-domain flag**: no  
**Investigation direction**: Add an explicit `Unknown` branch that maps to `ExitFailure` (or a new `ExitUnknown` code). Separate the error message for true timeout (`Progressing`) from connectivity/unknown errors. For `Suspended` (app intentionally paused by an operator), consider warning the user rather than failing.

---

## Finding 8: watchSingleApp pollOnce Fallback Provides a Single Stale Snapshot

**Severity**: Medium  
**Domain**: Go  
**Affected scenario**: gRPC stream failure mid-sync (network hiccup, ArgoCD pod restart)  
**Root cause hypothesis**: When `WatchApplication` returns an error or the channel closes mid-sync, `watchSingleApp` falls back to `pollOnce` (`orchestrator.go:157–169`). `pollOnce` calls `GetApplication` once, calls `updateFn` with whatever it receives, and returns. If `GetApplication` also fails (e.g. ArgoCD is restarting), `updateFn` is called with a zeroed `Result` (no health, no sync status). The zeroed result's `SyncStatus == ""` and `Health == ""` fall into the `default` branch of `watchApps` exit-code logic, producing `ExitTimeout`. The user gets no indication that the watch failed mid-stream; they just see a timeout with a blank status line.  
**Evidence**:  
- `orchestrator.go:154–169` — single `pollOnce` on any stream error; no retry.  
- `orchestrator.go:320–336` — `pollOnce` calls `updateFn(Result{App: name})` (zeroed) on error.  
- `orchestrator.go:139–141` — zeroed result: `Health == ""` and `SyncStatus == ""` → `default` → `ExitTimeout`.  
**Cross-domain flag**: no  
**Investigation direction**: Replace the single `pollOnce` with `pollUntilGone` (already implemented for `waitForDisappear`) or a bounded `pollWithRetry` that retries `GetApplication` every `DefaultPollInterval` until success or `ctx` cancels. Log a warning to the user that the watch stream failed and polling has taken over, so they understand the degraded mode.

---

## Finding 9: syncAfterMutation ReconcileFn Is Passed Through Request But Only Called at Sync Start

**Severity**: Low  
**Domain**: Go  
**Affected scenario**: `app enable` with a transient AppSet reconciliation failure  
**Root cause hypothesis**: `ReconcileFn` is called once at the start of `OpEnable`/`OpDisable` (`orchestrator.go:43–55`). If the patch fails (`reconcile.go:48–52`), `SyncAndWait` logs a warning and continues into `watchApps`, which will then block for the full timeout waiting for apps that will never appear (because the ApplicationSet was never reconciled). There is no retry of the reconciliation.  
**Evidence**:  
- `orchestrator.go:43–46` — `ReconcileFn` called once; error is only logged, not returned.  
- `appsetreconcile/reconcile.go:45–56` — `Trigger` returns an error on any patch failure; the error is silently downgraded to a warning.  
**Cross-domain flag**: no  
**Investigation direction**: Surface the `ReconcileFn` error to the caller rather than swallowing it — or at minimum retry once with a brief backoff before giving up. If reconciliation fails, return early with a clear error rather than waiting for the full timeout.
