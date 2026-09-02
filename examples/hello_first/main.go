package main

import (
	"examples/share_actor"
	"examples/testpb"

	"github.com/chenxyzl/grain"
)

func main() {
	//warning: etcd url
	system := grain.NewSystem("hello_first", "0.0.1", []string{"127.0.0.1:2379"})
	system.Start()
	//create actor and return actorRef
	actorRef := system.Spawn(func() grain.IActor { return &share_actor.HelloActor{} })
	actorRef.Tell(&testpb.Hello{Name: "hello tell"})
	reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, &testpb.HelloAsk{Name: "hello ask"})
	if err != nil {
		panic(err)
	}
	system.Logger().Info("reply:", "message", reply)
	//waiting ctrl+c
	system.WaitStopSignal(nil, nil)
}
