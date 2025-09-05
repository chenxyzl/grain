# Grain
- default distributed actor framework.
- easy to use. (only a etcd needs to be provided)
- highly scalable.
- fast. (run examples/benchmark_test/actor_test)
- support reentrant request.
- support pub/sub(local and global)

# Install
- go get github.com/chenxyzl/grain/...

# Example:

## tell & request
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
case *testpb.HelloRequest: //request-reply
x.Logger().Info("recv request", "message", context.Message())
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
actorRef.Send(&testpb.Hello{Name: "hello tell"})
//request
reply, err := grain.NoReentryRequest[*testpb.HelloReply](actorRef, &testpb.HelloRequest{Name: "hello request"})
if err != nil {
panic(err)
}
system.Logger().Info("reply:", "message", reply)
//waiting ctrl+c
system.WaitStopSignal()
}
```
## cluster mode
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
grain.WithConfigRequestTimeout(time.Second*1))
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
actorRef.Send(&testpb.Hello{Name: "hello tell, times:" + strconv.Itoa(times)})
//request
system.Logger().Info("request: ", "target", actorRef)
reply, err := grain.NoReentryRequest[*testpb.HelloReply](actorRef, &testpb.HelloRequest{Name: "xxx, times:" + strconv.Itoa(times)})
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
BenchmarkSendOne-16              4021477               280.6 ns/op
BenchmarkSendMore-16            11262148               133.4 ns/op
BenchmarkRequestOne-16            373494              2959 ns/op
BenchmarkRequestMore-16          1926564               645.1 ns/op
PASS
```