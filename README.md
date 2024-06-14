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
- warning: running etcd first
- then: define actor first:
``` go file:hello_actor.go
package share_actor

import (
    "examples/testpb"
    "fmt"
    "github.com/chenxyzl/grain/actor"
    "google.golang.org/protobuf/proto"
)

type HelloActor struct{ actor.BaseActor } //warning: inherit actor.BaseActor

func (x *HelloActor) Started() { x.Logger().Info("Started") }
func (x *HelloActor) PreStop() { x.Logger().Info("PreStop") }
func (x *HelloActor) Receive(context actor.Context) {
    switch msg := context.Message().(type) {
    case *testpb.HelloRequest: //request-reply
        x.Logger().Info("recv request", "message", context.Message())
        context.Reply(&testpb.HelloReply{Name: "reply " + msg.Name})
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
    "github.com/chenxyzl/grain/actor"
)

func main() {
    //warning: etcd url
    //config
    config := actor.NewConfig("hello_first", "0.0.1", []string{"127.0.0.1:2379"})
    //create system
    system := actor.NewSystem[*actor.ProviderEtcd](config)
    //start
    system.Start()
    //create a actor and return a actorRef
    actorRef := system.Spawn(func() actor.IActor { return &share_actor.HelloActor{} })
    //tell
    actor.NoEntrySend(system, actorRef, &testpb.Hello{Name: "hello tell"})
    //request
    reply, err := actor.NoEntryRequestE[*testpb.HelloReply](system, actorRef, &testpb.HelloRequest{Name: "hello request"})
    if err != nil {
    panic(err)
    }
    system.Logger().Info("reply:", "message", reply)
    //waiting ctrl+c
    system.WaitStopSignal()
}
```
## cluster
- warning: running etcd first
- then: define actor first(same as above, ignore)
### cluster server
``` go
package main

import (
	"examples/share_actor"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/utils"
)

func main() {
	utils.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		actor.WithKind("player", func() actor.IActor { return &share_actor.HelloActor{} }))
	//system
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Start()
	system.Logger().Warn("system starting")
	system.Logger().Warn("system started successfully")
	//wait ctrl+c
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}

```
### cluster client
``` go
package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/utils"
	"time"
)

func main() {
	utils.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		actor.WithRequestTimeout(time.Second*1))
	//new system
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Logger().Warn("system starting")
	system.Start()
	system.Logger().Warn("system started successfully")
	//get a remote actorRef
	actorRef := system.GetRemoteActorRef("player", "123456")
	//tell
	actor.NoEntrySend(system, actorRef, &testpb.Hello{Name: "hello tell"})
	//request
	system.Logger().Info("request: ", "target", actorRef)
	reply, err := actor.NoEntryRequestE[*testpb.HelloReply](system, actorRef, &testpb.HelloRequest{Name: "xxx"})
	if err != nil {
		panic(err)
	}
	system.Logger().Info("reply:", "message", reply)

	//wait ctrl+c
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}

```

## More examples
for more examples, please read grain/examples

## Benchmark
``` benchmark
goos: windows
goarch: amd64
pkg: examples/benchmark_test/actor_test
cpu: Intel(R) Core(TM) i7-9700K CPU @ 3.60GHz
BenchmarkSendOne
BenchmarkSendOne-16        	 1941168	       580.2 ns/op
BenchmarkSendMore
BenchmarkSendMore-16       	11428495	       201.0 ns/op
BenchmarkRequestOne
BenchmarkRequestOne-16     	  250057	      4390 ns/op
BenchmarkRequestMore
BenchmarkRequestMore-16    	 1517421	       856.9 ns/op
```