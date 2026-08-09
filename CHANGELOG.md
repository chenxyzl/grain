# Changelog

## v1.2.1

Correctness follow-up to v1.2.0. No API changes — a drop-in upgrade.

### 🐞 Correctness / concurrency fixes
- **ringbuffer lost-wakeup deadlock fixed** (the headline fix). v1.2.0's
  "`Pop` only signals when the buffer was actually full" optimization was
  **wrong**: a single drain loop pops many items in a row and frees many slots,
  but the buffer is full only on the *first* pop — so only one blocked sender was
  woken and every other queued sender stayed parked in `Push` forever. Because
  the drainer is only re-armed *after* `Push` returns, the whole actor then
  deadlocked (observed as thousands of goroutines stuck in `ringbuffer.Push`
  under load, e.g. `BenchmarkSendMore`). `Pop` now wakes a waiter whenever one is
  blocked and a slot was freed, tracked via a `waiters` counter (the common
  no-waiter fast path still skips the notify). Added a scheduler-model regression
  test that deadlocks on the old code and passes on the fix.
- **etcd watch could lose events between the initial Get and the Watch.** The
  three watchers (event-stream, member nodes, node ext-data) did `Get(prefix)`
  then `Watch(prefix)` with no start revision, so changes made in the gap were
  lost (neither in the Get snapshot nor the Watch stream). They now anchor the
  watch to the snapshot revision — `Watch(..., WithRev(rsp.Header.Revision+1))` —
  the canonical etcd atomic snapshot-then-watch pattern (no lost or duplicated
  events).

### 🧹 Internal / refactor
- **eventStream decoupled from etcd**: the `iProvider` interface now exposes
  semantic event-stream methods (`registerEventStream` / `unregisterEventStream` /
  `watchEventStream` with a neutral `watchOp`), so `eventStream` no longer imports
  `clientv3` / `mvccpb`. `getEtcdClient` / `getEtcdLease` removed from `iProvider`.
- **system listen address cached**: `getAddr()` is now a field read instead of an
  interface call on every send.
- Removed a leftover `net/http/pprof` debug server from the benchmark example.

## v1.2.0

> ⚠️ **Breaking changes** — this release changes several public APIs and raises the
> minimum Go version. Read "Breaking changes" before upgrading.

Robustness, correctness, performance, and security overhaul of the actor runtime —
a rewritten reentrancy engine, a lower-allocation Ask path, and a full
dependency/toolchain upgrade to 0 reachable vulnerabilities.

### ⚠️ Breaking changes
- **Requires Go >= 1.26** (pulled in by `go.etcd.io/etcd/client/v3` v3.7.x).
- **`Ask` now returns an error**: `Ask` / `NoReentryAsk` return
  `(msg, *message.ErrCode)` instead of panicking on failure (the old `AskE` was
  merged away). Runtime failures (timeout / remote / type-mismatch) are returned,
  not panicked.
- **Reentrant Ask semantics**: `x.Ask(target,msg)` (on `BaseActor`, inside
  `Receive`) is the reentrant form; `grain.NoReentryAsk[T]` is for non-actor
  callers. Reentrancy is now **general** (Akka/Orleans style): while an actor is
  blocked in `Ask`, other queued messages may run, so actor state can change
  across an `Ask`.
- **`BaseActor.Send` removed** — use `ref.Tell(msg)` (to any actor) or
  `x.Self().Tell(msg)` (to self).
- **`ActorRef.NoReentryAsk` / `NoReentryAskE` methods removed** — use the
  top-level `grain.NoReentryAsk[T]`.
- **error-code registry (`RegisterCode`) removed** — `ErrCode` stays `{Code, Des}`;
  classify with your own consts.

### 🐞 Correctness / concurrency fixes
- Mailbox **lost-wakeup race** fixed (a message could stall until the next send).
- **Concurrent-spawn process crash** fixed: idempotent `getOrAdd` replaces the
  duplicate-id panic on the write_stream / cluster-actor spawn paths.
- **Reentrancy rewritten** from the fragile `runningMsgId`+inline trick to a
  **turn-token** scheduler — strictly single-threaded, breaks a→b→a and self-Ask
  deadlocks, no data race under `Forward`. See `docs/reentrancy.md`.
- **Shutdown deadlock** fixed: poison is now a non-blocking control flag; it no
  longer blocks under the registry shard lock.
- **Bounded-blocking mailbox** with `Close()` wakeup replaces the unbounded
  auto-growing buffer; a sender to a stopped actor is woken instead of stuck.
- **Cross-machine addressing** fixed: registers a reachable inner IP
  (RFC1918-preferred, deterministic), falls back to `127.0.0.1`; no longer
  advertises `[::]`.
- **Rendezvous hashing** deterministic tie-break (no split ownership across nodes).
- `PreStop` only runs when `Started()` completed; remote-`Poison` waits for
  in-flight handlers; UUID clock-rollback guard; plus several nil/edge fixes.

### ⚡ Performance
- **Ask allocations 18 → ~5, ~50% faster**: reply is now a correlation-id +
  pending table (no throwaway reply actor), and the timeout `time.Timer` is pooled.
- **provider version → atomic**: cluster-send hot path drops a per-message
  `RWMutex.RLock` (~19× faster concurrent version read in microbenchmark).
- **ringbuffer**: `Pop` only signals when the buffer was actually full; `intFnv32`
  no longer allocates.

### 🔒 Security
- Upgraded all deps to latest: **etcd v3.7.1, grpc v1.82.1, protobuf v1.36.11**,
  golang.org/x/*, genproto; toolchain **Go 1.26.5**.
- `govulncheck`: **0 reachable vulnerabilities**.

### 📚 Docs
- New `docs/reentrancy.md` (turn-token reentrancy internals).
- README / README_ZH updated: requirements, messaging APIs, reentry example,
  benchmark build/run instructions.
