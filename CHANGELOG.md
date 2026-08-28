# Changelog

## v1.2.3

`Ask` becomes a generic method now that Go 1.27 allows methods to declare their
own type parameters. This is what the `// wanted ... but golang not support`
comment on `NoReentryAsk` had been waiting for — and it turns a latent
error-swallowing bug into a compile-time-checked reply type.

This release also fixes three pre-existing production-triggerable faults found in a
full-framework audit: a cluster-wide deadlock, a crash on peer disconnect, and a
data race. **If you run a cluster, upgrade for those rather than for `Ask[T]`.**

### 💥 Crash / deadlock / race fixes
All three were reproduced with a test before fixing, and each fix ships with a
regression test that was verified to fail against the old code.

- **Deadlock: a cluster membership change concurrent with any actor spawn/stop could
  wedge a whole registry shard, permanently.** `ConcurrentMap.IterCb` holds a shard
  read lock while invoking its callback, and `clusterMemberChanged` called
  `system.Poison` from inside it → `registry.get` → the *same* shard. `sync.RWMutex`
  read locks are not reentrant once a writer is queued, so a concurrent
  `registry.add`/`remove` blocked the nested read behind that writer while the outer
  read lock was still held — and it was never released, so every later lookup or
  spawn on that shard blocked too. In a live cluster the trigger (etcd membership
  change + an actor starting or stopping) is routine.
  - `registry.rangeIt` now snapshots and invokes the callback **outside** the lock, so
    re-entry is safe by construction. As a bonus the per-actor rendezvous hashing no
    longer runs under the shard lock.
  - `clusterMemberChanged` now calls `v.poison()` on the process it already holds
    instead of `x.Poison(self)`. That also avoids `Poison`'s "not in registry →
    `tell(ref, poison)`" fallback, which for a cluster ref would route through
    `ensureClusterKindActorExist` and **spawn the actor just to kill it**.
  - `IterCb` and `RWMap.Range` now document that the callback must not touch the map.
- **Crash: one peer disconnecting could take the whole process down.**
  `remote/stream_server.go`'s `Listen` had a `case status.Code(err) > 0` arm that
  logged without returning. `codes.Canceled` is 1, so every code ≥ 2 — notably
  `Unavailable` (14), what grpc reports when a peer process dies or its TCP
  connection drops — fell through to `recvEnvelope(msg)` with `msg == nil`. Measured
  before the fix: **~460k iterations/sec** on a dead stream, each delivering a nil
  envelope, and `system.RecvEnvelope` dereferenced it. That panic runs on a grpc
  handler goroutine, which nothing recovers.
  - `Listen` now returns on *every* `Recv` error (EOF/Canceled as success, anything
    else as an error), and never passes a nil envelope on.
  - `RecvEnvelope` now rejects a nil envelope and uses the nil-safe `Get*` accessors
    throughout — it is the entry point for data from another node, so it is treated
    as untrusted.
- **Data race in `ScheduleRepeated`.** `schedule.go` read the timer's `state`
  non-atomically in the fired callback while the cancel func wrote it with
  `atomic.SwapInt32`; the race detector flags it on any repeated-schedule + cancel.
  Now an `atomic.LoadInt32`. The callback also re-checks before `t.Reset`, so a
  cancel racing with the callback no longer leaves the timer armed for one more
  interval after `Stop()`.

### 🐞 Correctness fixes from the same audit
- **A remote `Tell` gave you a non-nil but empty `ctx.Sender()`.** `stream_write`
  writes `""` when there is no sender, and `newActorRefFromAID("")` returns a NON-nil
  ref whose kind/name/addr are all empty. So `if ctx.Sender() != nil` — used by the
  framework itself and by essentially every handler — was a false positive for every
  remote Tell, and `ctx.Reply()` on such a message failed with the misleading
  `"actor kind not in cluster"`. The sender is now nil when the AID is empty, and an
  envelope with an empty target is rejected instead of routed.
- **`Poison` on a dormant cluster actor activated it just to kill it.**
  `tellWithSender` called `ensureClusterKindActorExist` unconditionally, so a poison
  ran the grain's full `Started()` (cluster registration, state load) and then
  `PreStop()` immediately — which for a grain that persists in `PreStop` can write
  empty or just-loaded state over the real thing. It also made `sendToLocal`'s
  existing "ignore poison if proc not found" guard unreachable for cluster kinds.
  Poison no longer activates anything.
- **Messages sent during `stop()` vanished without a dead letter.** `doStop` only
  checks the mailbox is empty *before* `PreStop`, and `procStatus` stays `running`
  for the whole of `PreStop` (user code, so arbitrarily long). In that window a sender
  got `PushOK` while `schedule()`'s CAS failed, so nothing drained them — and
  `rb.Close()` keeps queued items poppable rather than rejecting them, so they were
  dropped with no dead letter and no log. `stop()` now drains the remainder to dead
  letters after closing the ring.
- **An `Ask` into a saturated or stopped mailbox blocked for the full `askTimeout`**,
  while an `Ask` to a *missing* actor failed instantly. `toDeadLetter` now replies to
  a waiting Ask, so both fail promptly.
- **New cluster grains could be activated after shutdown had finished.** grpc is
  stopped only after the actors are drained, so inbound envelopes kept arriving and
  `ensureClusterKindActorExist` happily spawned actors that nothing would ever poison
  — their `PreStop`, and any state persistence in it, never ran. A `draining` flag set
  at the start of `stopActors` now refuses on-demand activation.
- **`errActorNotFound` / `errAskNotRunning` were shared mutable singletons** that
  escape into user code (one is Tell'd to the asking actor and comes back out of Ask).
  `*ErrCode` has exported fields, so one caller doing `err.Des += ...` corrupted it
  process-wide. Both are now built per use; they are on cold paths.
- **`ForceStop` could block its caller forever.** `forceCloseChan` has capacity 1 and
  only `WaitStopSignal` drains it, so a second `ForceStop` — or one issued when
  `WaitStopSignal` was never called or has already returned — parked the caller
  permanently. Callers include the etcd keepalive and member-watch goroutines. Now a
  non-blocking send.
- **Cluster-register retry slept 400ms after its final attempt** before giving up,
  with the actor's `Started()` blocked on it. The backoff now only happens between
  attempts. A failed *de*-registration is no longer swallowed by an empty `if` either
  — it returns an error and is logged, since it leaves a stale routing entry.

### 🔌 etcd provider: liveness and lifecycle
- **A terminated watch froze the node's view of the cluster, silently and
  permanently.** All three watch loops were plain `for v := range wch` and never
  checked `v.Canceled` / `v.Err()`. clientv3 signals a terminal watcher failure
  (compacted revision — likely here, since the watch is anchored with
  `WithRev(rev+1)` — auth revocation, server-side cancel) with one Canceled response
  and then closes the channel: the loop just exited and nothing was logged. The member
  watch kept routing cluster actors to nodes that had left and never learned of new
  ones. All three now detect it; the member watch escalates to `ForceStop`, because
  routing on a stale member set is worse than being down (the same escalation the
  lease-expiry path already used). Automatic re-establishment is a follow-up, marked
  with a TODO.
- **`start()` leaked on every error path** — the etcd client, the lease, and the
  keepalive goroutine were all left running, and `x.cancelFunc` was never called
  anywhere in the codebase. Worse, if `watch()` failed, `register()` had already
  published this node's member key, so peers routed real traffic for up to the lease
  TTL to a node that never came up. A shared `releaseEtcd()` now cancels, revokes
  (which deletes the lease-bound member key) and closes on both the failure path and
  `stop()`.
- **`stop()` signalled teardown by assigning `x.system = nil`**, an unsynchronized
  write to an interface field that the keepalive goroutine reads — a data race, with a
  nil-call window between its check and its use. Replaced with an `atomic.Bool`.
- **One malformed member value bricked every node trying to join.** `parseWatch`
  returned the json error *after* already handling it, and `watch()`'s initial load
  propagated it up through `start()` into a panic in `system.Start()` — while the live
  watch path discarded the identical error with `_ =`. It now logs, drops that node,
  and returns nothing.
- Dropped `WithPrevKV()` from all three watches: `PrevKv` was never read, so it was
  pure server-side cost and bandwidth. Demoted the per-member-change log from `Warn`
  to `Info`.
- `setTxn` / `removeTxn` return `false` both for "lost the race" and "etcd failed",
  which the caller cannot tell apart, so the etcd error is now logged at the point of
  failure. Without it, a brief etcd outage during `register()` looked exactly like
  "node ids 1..1023 are all taken" and the real cause appeared nowhere.

### 💣 Breaking API changes
Done in one step rather than staged behind deprecations, by request.

- **`BaseActor` no longer embeds `ActorRef`** — it holds a named `self` field. The
  embedded form made every user actor implicitly *be* an `ActorRef`, with three
  consequences: `x.Tell(msg)` read like "tell someone" while meaning "tell myself";
  `&MyActor{}` compiled anywhere an `ActorRef` was expected, silently passing an actor
  where a reference belonged; and before `_init` the embedded interface was nil, so
  touching it from a constructor or a field initializer nil-panicked with no
  explanation.
  - **Migration:** anything reference-shaped now goes through `Self()` —
    `x.Tell(m)` → `x.Self().Tell(m)`, `x.GetId()` → `x.Self().GetId()`, likewise
    `GetKind` / `GetName` / `GetDirectAddr`. `Self()`, `GetSystem()`, `Logger()`,
    `Ask[T]` and `ScheduleSelf*` are unchanged.
  - Using any of them before the actor is spawned now panics with a message naming the
    mistake, instead of dereferencing nil.
  - Nothing in this repo used the promoted methods, so the framework, examples and
    tests needed no changes — but your own actors may.
- **`SpawnNamed` returns `(ActorRef, error)`** and reports `ErrNameExists` instead of
  panicking. `registry.add`'s `panic("duplicated process id")` killed the whole process
  for what is either a caller mistake or a benign race (respawning a named actor after
  a crash; two goroutines racing to create it).
  - Reference points: protoactor-go — the closest sibling design — returns
    `(*PID, error)` with `ErrNameExists`; Akka throws a *catchable*
    `InvalidActorNameException`; Orleans has no such failure at all, because grains are
    virtual and activation is idempotent. This framework's *cluster* kinds already
    behave the Orleans way via `ensureClusterKindActorExist`, so only the explicit
    named spawn needed fixing, and in Go that means an error.
  - `Spawn` keeps its error-free signature: the name is a generated uuid, so a
    collision would be a framework invariant violation, and it panics as such.
- **`WithOptsInboxSize` / `WithOptsInboxMaxSize` renamed to `WithOptsMailboxSize` /
  `WithOptsMailboxMaxSize`.** Everything else in the codebase — the fields, the
  constants, the docs, the dead-letter reasons — says "mailbox"; "Inbox" appeared only
  in these two option names.

### 🧽 API and hygiene
- **`ISystem` is now a sealed interface with its internals grouped.** The nine
  unexported methods (`getAddr`, `getSender`, `getConfig`, `getRegistry`, `nextSnId`,
  `registerAsk`, `cancelAsk`, `getAddrHash`, `getProvider`) moved into an embedded
  unexported `iSystem`, so godoc shows the public contract plus one embedded name
  instead of nine internals inline. Behaviour for callers is unchanged: unexported
  methods were never reachable from another package, and embedding them keeps `ISystem`
  *sealed* — no outside type can implement it, which leaves the framework free to add
  methods without breaking anyone. No second accessor was needed on `ActorRef`:
  internal code calls the hooks straight off any `ISystem` value.
- **No exported symbol returns or takes an unexported type any more.**
  - `iScheduler` → **`IScheduler`**: `GetScheduler()` is documented in the README, so
    returning an unexported interface meant callers could invoke its methods but never
    declare a variable, write a helper, or mock it.
  - `iProducer` → **`Producer`**: appears in `Spawn`, `SpawnNamed` and
    `WithConfigKind`. A func literal satisfied it either way, so existing call sites
    are unaffected.
  - **`GetProvider()` is gone** (now internal `getProvider()`). It returned the
    unexported `iProvider`, whose exported methods were therefore callable only by
    accident. The genuinely user-facing ones are now first-class on `ISystem`:
    `GetNodeId`, `GetNodeExtData`, `SetNodeExtData`, `RemoveNodeExtData`,
    `WatchNodeExtData`. **Migration:** `system.GetProvider().GetNodeExtData(k)` →
    `system.GetNodeExtData(k)`.
- **On-demand grain activation moved from `tellWithSender` into `sendToLocal`**, after
  the poison check that was already there. Same behaviour, but the message-type check
  happens once on that path instead of twice, and the routing layer no longer
  special-cases a message type. A failed activation now also reports
  `errActorNotFound` to a waiting Ask instead of only logging.
- **`WithConfigGrpcDialOptions` now APPENDS instead of replacing.** It used to
  overwrite the slice, silently dropping the default `insecure.NewCredentials()`
  seeded in `newConfig` — so adding one unrelated dial option made `grpc.NewClient`
  fail with "no transport security set", visible only as `streamWriteActor` logging
  and poisoning itself. Same for call options.
- **`WithConfigCallDialOptions` is deprecated in favour of `WithConfigGrpcCallOptions`**
  — it takes `grpc.CallOption`s and has nothing to do with dialing. The old name still
  works and forwards.
- **`DeadLetter` gains `Owner`**, the actor whose mailbox actually rejected the
  message. It differs from `Target` on the outbound path: `sendToCluster` builds the
  context with the *remote* target but pushes into the local `write_stream` actor's
  mailbox, so previously an overflow gave no way to tell which of the two was
  saturated. The fallback WARN log now names both. Additive — existing handlers keep
  compiling.
- `Spawn`'s generated actor name used `strconv.Itoa(int(uuid.Generate()))`, which
  truncates on a 32-bit build and could collide names. Now `strconv.FormatUint`.
- `ghelper.StackTrace` rewritten on `runtime.Callers` + `CallersFrames`: the old loop
  re-walked the stack per frame, concatenated O(n²), reported the *outer* function for
  inlined frames, and its `"/src/"` path trim never matched in a module build (so
  project files were logged with full absolute paths). Truncation is now marked
  instead of silent.
- `event_stream`'s `if actorRef == nil` was dead — `newActorRefFromAID` never returns
  nil, it returns a ref with empty fields — so a malformed actor id was accepted
  silently. It now validates what actually indicates a bad id.
- Removed dead code: `config.addr` (never read or written, and the only reason `net`
  was imported), `config.GetNodeId()`, and the unused `changed` result of
  `getRemoteAddrCache` together with its unreachable `vCache == nil` tail (that
  function also now uses `defer` for its lock instead of three manual unlocks).
- Removed `WithOptsRegisterToCluster` / `WithOptsUnRegisterFromCluster`: zero call
  sites, and **impossible to call from outside the package** — the callback took
  `iProvider` and `*config`, both unexported, so no external closure could be written.
  Making them a real extension point requires exporting those types first.


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

### ⛔ `Ask` is now restricted to the actor's running phase
**Behavior change — code that Asks from `Started()` or `PreStop()` starts failing.**
It sends nothing and returns the preset `errAskNotRunning`
(`message.CodeAskNotRunning`) immediately, instead of blocking.

Why `Started()`: reentrancy is deliberately off there — `yieldTurn` skips the
successor-drainer handoff so no handler runs against half-initialized state — which
also means the actor cannot answer any incoming request in that window. A reply
still arrives via the pending table, so a plain a→b Ask used to work; but if the
reply required the peer to send this actor a message first (an a→b→a cycle), that
message landed in a mailbox nobody was draining and both sides waited out
`askTimeout`. Whether a given Ask depends on that is **not decidable at call time**,
so rather than predict it — or report it late, after the timeout already elapsed —
the phase itself is disallowed. The error therefore lands at the earliest possible
point, with no waiting and no false positives.

Why `PreStop()`: see the double-`PreStop` fix below.

- **Migration:** move the Ask into a normal handler. For "Ask at startup", `Tell`
  self from `Started()` and Ask when handling that message. `Tell` is unrestricted in
  every phase, so shutdown notifications belong there too. This is a **runtime**
  error, not a compile error, so grep for `Ask` inside `Started()`/`PreStop()` when
  upgrading.
- The gate is an **allow-list** (`life == lifeStarted`) rather than a deny-list on
  specific phases, so a lifecycle phase added later is refused by default instead of
  silently permitting a blocking Ask. `lifeStopping` turned out to be exactly such a
  phase.
- `examples/hello_reentry` was Asking from `Started()`: the flagship reentrancy demo
  actually produced two `ask reply timeout` errors instead of demonstrating
  reentrancy. The Ask now runs from a normal handler and the full a→b→a cycle
  completes sub-second.
- Not covered: `NoReentryAsk` passes no turn, so the framework cannot tell which
  phase it is called from. Calling it inside an actor was already wrong (it never
  yields the turn) and remains so.
- `message.CodeAskNotRunning` (`-3`) is a new code, so this is distinguishable from a
  timeout programmatically. Nothing in the tree inspected `ErrCode.Code` before, so
  no existing check changes meaning.
- The misleading `yieldTurn` comment ("still gets its reply ... so it doesn't
  deadlock" — false for a cycle) is corrected, and the rule plus the reasoning about
  *when* it is decidable is written up in **docs/reentrancy.md §九**.

### 🐞 `PreStop()` could run twice
A turn-yielding call inside `PreStop()` — an `Ask`, back when that was allowed — let
`PreStop` execute **twice**. Confirmed by test before the fix, count was 2.

`stop()` holds the turn while calling `PreStop`, but `procStatus` only becomes
`stopped` in `stop()`'s **defer**, i.e. after `PreStop` returns. So when `PreStop`
released the turn, the successor drainer spawned by `yieldTurn` found
`procStatus == running`, an empty mailbox and `inflight == 0`, re-entered
`doStop() -> stop()`, passed the top guard, still saw `life == lifeStarted` and called
`PreStop` again. Any cleanup in `PreStop` that is not idempotent ran twice.

- Fixed by a new `lifeStopping` lifecycle value: `stop()` advances `life` **before**
  invoking `PreStop`, so a re-entrant `stop()` skips it. This also removes `PreStop`
  from the Ask-allowed phase, which is why `Ask` is refused there.
- The `lifecycle` type's doc claimed "Started/PreStop phase" while having no PreStop
  value; it now does.
- Regression tests: `TestPreStopRunsOnceWhenItYieldsTheTurn` drives the low-level
  yield path (still reachable now that `Ask` refuses), and
  `TestAskFromPreStopIsRejected` covers the `Ask` route. Both fail with count 2 if the
  `lifeStopping` assignment is removed.
- New test `TestAskFromStartedIsRejected` pins the rule: it asserts the code, that
  nothing was delivered to the target, and that no `askTimeout` wait occurs.
  `TestSelfAskDoesNotDeadlock` covers the working from-a-handler case.

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
