# Grain[[中文文档]](https://github.com/chenxyzl/grain/tree/main//README_ZH.md)
- default distributed actor framework.
- easy to use. (only etcd needs to be provided)
- highly scalable.
- fast. (run examples/benchmark_test/actor_test)
- support reentrant ask.
- support publish/subscribe(local and cluster)
- support schedule

# Install
- go get github.com/chenxyzl/grain/...

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
system.WaitStopSignal()
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
system.WaitStopSignal()
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
system.WaitStopSignal()
//
system.Logger().Warn("system stopped successfully")
}

```

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
build benchmark exec
``` bash
  cd examples/benchmark_test/actor_test
  GOOS=windows GOARCH=amd64 go test -c -o bench-windows-amd64.exe ./...
```
run
``` cmd
  bench-windows-amd64.exe -test.bench=.
```
result
``` benchmark result
goos: windows
goarch: amd64
pkg: examples/benchmark_test/actor_test
cpu: Intel(R) Core(TM) i7-9700K CPU @ 3.60GHz
BenchmarkSendOne-16              3841665               279.6 ns/op
BenchmarkSendMore-16            11471660               128.2 ns/op
BenchmarkAskOne-16                335313              3230 ns/op
BenchmarkAskMore-16              1924821               692.6 ns/op
PASS
```