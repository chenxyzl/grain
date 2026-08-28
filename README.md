# Grain[[中文文档]](https://github.com/chenxyzl/grain/tree/main//README_ZH.md)
- default distributed actor framework.
- easy to use. (only etcd needs to be provided)
- highly scalable.
- fast. (run examples/benchmark_test/actor_test)
- support reentrant ask. (an actor can Ask while handling a message, even in an a->b->a cycle, without deadlock; the actor stays single-threaded)
- support publish/subscribe(local and cluster)
- support schedule

# Requirements
- Go >= 1.27 (required by the generic method `BaseActor.Ask[T]`)
- an etcd cluster (used for member discovery / cluster addressing)

# Install
- go get github.com/chenxyzl/grain/...

# Messaging APIs at a glance
- `ref.Tell(msg)` — fire-and-forget send to an actor (method on `ActorRef`).
- `ctx.Reply(msg)` — reply to the current request (inside `Receive`).
- `x.Ask[T](target, msg)` — **reentrant** blocking request/reply, usable from inside
  `Receive` (method on `BaseActor`). While it waits, the actor yields its turn so
  other messages (including a reply looping back to itself) are processed, then
  resumes — no deadlock, still single-threaded. Returns `(T, *message.ErrCode)`.
  `T` is the expected reply type and must be written explicitly, as it cannot be
  inferred from the arguments.
- `grain.NoReentryAsk[T](target, msg)` — blocking request/reply for **non-actor**
  callers (e.g. `main`, a client). NOT reentrant — do not call from inside an
  actor's `Receive`/`Started`. Returns `(T, *message.ErrCode)`.

> All request/reply errors are returned as `*message.ErrCode` (nil = success),
> never panicked; runtime failures (timeout / remote error / type mismatch) are
> classified there.

> ⚠️ **`x.Ask` is only allowed while the actor is running** — from a normal handler.
> From `Started()` or `PreStop()` it sends nothing and returns
> `message.CodeAskNotRunning` immediately. In `Started()` reentrancy is off (a
> handler must not run against half-initialized state) so the actor cannot answer
> incoming requests; in `PreStop()` blocking would re-enter the stop path. To Ask at
> startup, self-`Tell` from `Started()` and Ask when handling that message. `Tell` is
> unrestricted in every phase. See docs/reentrancy.md §九.

# Example:

## examples/first(tell & ask/reply)
warning: running etcd first
- define actor:
``` go file:hello_actor.go
package share_actor

import (
"examples/testpb"
"fmt"

"github.com/chenxyzl/grain"
"google.golang.org/protobuf/proto"
)

type HelloActor struct{ grain.BaseActor }

func (x *HelloActor) Started() { x.Logger().Info("Started") }
func (x *HelloActor) PreStop() { x.Logger().Info("PreStop") }
func (x *HelloActor) Receive(context grain.Context) {
switch msg := context.Message().(type) {
case *testpb.HelloAsk: //ask-reply
x.Logger().Info("recv ask", "message", context.Message())
context.Reply(&testpb.HelloReply{Name: "reply hello to " + context.Sender().GetName()})
case *testpb.Hello: //tell
x.Logger().Info("recv tell", "message", context.Message())
default:
panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
}
}
```

- use:
``` go
package main

import (
"examples/share_actor"
"examples/testpb"

"github.com/chenxyzl/grain"
)

func main() {
//warning: etcd url
//create system
system := grain.NewSystem("hello_first", "0.0.1", []string{"127.0.0.1:2379"})
//start
system.Start()
//create a actor and return a actorRef
actorRef := system.Spawn(func() grain.IActor { return &share_actor.HelloActor{} })
//tell
actorRef.Tell(&testpb.Hello{Name: "hello tell"})
//ask
reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, &testpb.HelloAsk{Name: "hello ask"})
if err != nil {
panic(err)
}
system.Logger().Info("reply:", "message", reply)
//waiting ctrl+c
system.WaitStopSignal(nil, nil)
}
```
## examples/cluster
warning: running etcd first
warning: define actor(same as above, ignore)

- cluster server
``` go
package main

import (
"examples/share_actor"
"log/slog"

"github.com/chenxyzl/grain"
)

func main() {
grain.InitLog("./test.log", slog.LevelInfo)
//system
system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
grain.WithConfigKind("player", func() grain.IActor { return &share_actor.HelloActor{} }))
//start
system.Logger().Warn("system starting")
system.Start()
system.Logger().Warn("system started successfully")
//wait ctrl+c
system.WaitStopSignal(nil, nil)
//
system.Logger().Warn("system stopped successfully")
}

```
- cluster client
``` go
package main

import (
"examples/testpb"
"log/slog"
"strconv"
"time"

"github.com/chenxyzl/grain"
)

func main() {
grain.InitLog("./test.log", slog.LevelInfo)
//new system
system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
grain.WithConfigAskTimeout(time.Second*1))
//start
system.Logger().Warn("system starting")
system.Start()
system.Logger().Warn("system started successfully")
//get a cluster actorRef
actorRef := system.GetClusterActorRef("player", "123456")
if actorRef == nil {
panic("GetClusterActorRef failed")
}
//
go func() {
c := time.NewTicker(3 * time.Second)
times := 0
for range c.C {
times++
//tell
actorRef.Tell(&testpb.Hello{Name: "hello tell, times:" + strconv.Itoa(times)})
//ask
system.Logger().Info("ask: ", "target", actorRef)
reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, &testpb.HelloAsk{Name: "xxx, times:" + strconv.Itoa(times)})
if err != nil {
system.Logger().Error(err.Error())
}
system.Logger().Info("reply:", "message", reply)
}
}()

//wait ctrl+c
system.WaitStopSignal(nil, nil)
//
system.Logger().Warn("system stopped successfully")
}

```

## examples/hello_reentry (reentrant ask)
warning: running etcd first

An actor may `x.Ask[T]` another actor while handling a message. If the callee asks
back (a -> b -> a), it does NOT deadlock: while `A` waits, it yields its turn so a
successor drainer processes the message `B` sends back to `A`; once the reply
arrives, `A` resumes. The actor is always single-threaded.
``` go
func (x *HelloActorA) Receive(ctx grain.Context) {
switch ctx.Message().(type) {
case *testpb.HelloAskB2A:
// A is being asked by B while A itself is blocked in x.Ask(B) — reentrancy
ctx.Reply(&testpb.HelloReplyB2A{Name: "HelloReplyB2A"})
}
}

func (x *HelloActorB) Receive(ctx grain.Context) {
switch ctx.Message().(type) {
case *testpb.HelloAskA2B:
// reentrant ask back to A; does not deadlock
reply, err := x.Ask[*testpb.HelloReplyB2A](helloActorA, &testpb.HelloAskB2A{Name: "HelloAskB2A"})
_ = reply; _ = err
ctx.Reply(&testpb.HelloReplyA2B{Name: "HelloReplyA2B"})
}
}
```
> `x.Ask[T](...)` (on `BaseActor`, callable inside `Receive`) is the reentrant form.
> See docs/reentrancy.md for how the turn/handoff mechanism works.

## examples/pubsub
- subscribe event  
$system.Subscribe(ref ActorRef, message proto.Message)
- publish local event  
$system.PublishLocal(message proto.Message)
- publish cluster event  
$system.PublishGlobal(message proto.Message)
- unsubscribe event  
$system.Unsubscribe(ref ActorRef, message proto.Message)

## examples/schedule
- actor schedule once  
$actor.ScheduleSelfOnce(delay time.Duration, msg proto.Message)
- system schedule once  
$system.GetScheduler().ScheduleOnce($actorRef, `/*more params like above*/`)
- actor schedule repeat  
$actor.ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message)  
- system schedule repeat  
$system.GetScheduler().ScheduleRepeated($actorRef, `/*more params like above*/`)
- cancel schedule  
CancelScheduleFunc()


## More examples
for more examples, please read grain/examples

## Benchmark
> requires a local etcd on 127.0.0.1:2379 (the benchmark's NewSystem connects there).

build benchmark exec (`CGO_ENABLED=0` for a static, cross-compiled binary)
``` bash
  cd examples/benchmark_test/actor_test
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c -o bench-windows-amd64.exe .
```
run (the compiled test binary uses `-test.` prefixed flags; `-test.run=none` skips
normal tests so only benchmarks run)
``` cmd
  bench-windows-amd64.exe -test.run=none -test.bench=. -test.benchmem
```
result
``` benchmark result
goos: windows
goarch: amd64
pkg: examples/benchmark_test/actor_test
cpu: Intel(R) Core(TM) i7-10700KF CPU @ 3.80GHz
BenchmarkSendOne-32              4953276               225.0 ns/op            80 B/op          1 allocs/op
BenchmarkSendMore-32            21939768                72.99 ns/op           80 B/op          1 allocs/op
BenchmarkAskOne-32                803352              1523 ns/op             273 B/op          5 allocs/op
BenchmarkAskMore-32              5265314               246.6 ns/op           273 B/op          5 allocs/op
PASS
```