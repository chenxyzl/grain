# Changelog

## v1.2.3

`Ask` becomes a generic method now that Go 1.27 allows methods to declare their
own type parameters. This is what the `// wanted ... but golang not support`
comment on `NoReentryAsk` had been waiting for — and it turns a latent
error-swallowing bug into a compile-time-checked reply type.

### ⚠️ Behavior changes
- **Requires Go 1.27** (was 1.26). Declaring a generic method needs `-lang=go1.27`;
  only this module is affected, callers may stay on an older language version.
- **`BaseActor.Ask` is now `Ask[T proto.Message]`** and returns `(T, *message.ErrCode)`
  instead of `(proto.Message, *message.ErrCode)`. `T` appears only in the result so
  it cannot be inferred — write it explicitly:

  ```go
  // before
  reply, err := x.Ask(target, &pb.HelloAsk{})            // reply is proto.Message
  // after
  reply, err := x.Ask[*pb.HelloReply](target, &pb.HelloAsk{})
  ```

  `x.Ask[proto.Message](...)` reproduces the old signature, but see the fix below
  before relying on it.

### 🐞 Correctness fixes
- **`Ask` no longer reports a failure reply as success** (the headline fix).
  `awaitReply`'s type switch tested `case T` first, and with `T` bound to
  `proto.Message` that arm matched *everything* — including `*message.ErrCode` and
  `*message.Poison`, which are themselves proto messages. The later `case *ErrCode`
  / `case *Poison` arms were dead code. Consequences, all now fixed:
  - Asking an actor that does not exist returned `errActorNotFound` as a **successful
    reply** with `err == nil`, instead of an error.
  - An actor replying `ctx.Reply(message.WithErr(...))` was likewise seen as success.
  - Shutdown's `wakePendingAsks` handed the poison sentinel back as a successful
    reply instead of `"ask reply poisoned"`.

  The two sentinels are now matched **before** `case T`, so the contract holds for
  every `T` — `Ask[proto.Message]` included.
- **Reply type mismatch is now actually detected.** With `T` = `proto.Message`
  nothing could mismatch; the `msg type err, need:%v, now:%v` diagnostic was
  unreachable and its `need:` was empty (`proto.MessageName` on a nil interface
  returns `""`). With a concrete `T` the zero value is a typed nil and the message
  names both types.
- **`event_stream` no longer loses subscriptions to a race.** Two paths did
  `Get` → nil-check → `NewRWMap` → `Set` on `eventStreamMaps`: `registerEventStream`
  on the actor's goroutine and `parseWatchEventStream` on the provider's watch
  goroutine. Interleaved, the second `Set` replaced the map the first had already
  written a node into. Both now use the new atomic `RWMap.GetOrCreate`.
- **`NoReentryAsk(nil, msg)` no longer panics** on `target.GetSystem()`; the
  nil-target guard moved into the shared path and returns an `ErrCode`.

### ✨ Additions
- **`safemap.RWMap.GetOrCreate(key, create)`** — check-and-insert under one write
  lock, returning the same value to all concurrent callers.

### 🧹 Internal
- `BaseActor.Ask` and `NoReentryAsk` now share one `askImpl[T]`; the correlation-id
  /register/send/await sequence and the yield-before-send reasoning live in one
  place. `askImpl` takes the turn controller, so the only difference between the
  two entry points is whether a turn is yielded.
- `newProvider[T iProvider]()` and its `reflect` dependency are gone — the single
  call site constructs `&providerEtcd{}` directly.

## v1.2.2

Mailbox reworked from fixed-capacity blocking to non-blocking auto-growth, with
overflow routed to a dead letter. Removes the deadlock and goroutine-pileup risk
of blocking mailboxes.

### ⚠️ Behavior changes
- **Sending no longer blocks (no back-pressure).** The mailbox starts small and
  **grows on demand (doubling) up to a max capacity**; a full mailbox at max
  overflows to a **dead letter** instead of blocking the sender. This removes the
  bounded-blocking-mailbox deadlock (a→b→a) and the goroutine pile-up under load.
  A fast producer + slow consumer now **drops at the ceiling** rather than
  slowing the sender down.
- **Default mailbox size**: was a fixed `1024`; now **init `128`, max `4096`**
  (init chosen as the throughput/memory sweet spot; idle actors use ~2KB).
- **`WithOptsInboxSize` now sets the INITIAL capacity** (was the fixed capacity).
  New **`WithOptsInboxMaxSize`** sets the ceiling.

### ✨ Additions
- **Dead letters**: undeliverable messages (mailbox overflow, or a send to a
  stopped actor) are surfaced as `DeadLetter{Target,Sender,Message,MsgSnId,Reason}`.
  Configure a handler with **`WithConfigDeadLetter`**; defaults to a WARN log. A
  panic in the handler is recovered and logged.

### 🧹 Internal
- ringbuffer rewritten: no `sync.Cond`/waiters, `Push` never blocks and returns
  `PushOK`/`PushOverflow`/`PushClosed`; `grow()` doubles + linearizes (amortized
  O(1), zero steady-state allocation). This also retires the machinery behind the
  v1.2.1 lost-wakeup fix.
- `msg.ProtoReflect().Descriptor().FullName()` unified to `proto.MessageName` on
  the internal error/log paths.

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
