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
	//create actor and return actorRef
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
