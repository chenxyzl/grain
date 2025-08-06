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
	actor.NoReentrySend(system, actorRef, &testpb.Hello{Name: "hello tell"})
	//request
	reply, err := actor.NoReentryRequest[*testpb.HelloReply](system, actorRef, &testpb.HelloRequest{Name: "hello request"})
	if err != nil {
		panic(err)
	}
	system.Logger().Info("reply:", "message", reply)
	//waiting ctrl+c
	system.WaitStopSignal()
}
