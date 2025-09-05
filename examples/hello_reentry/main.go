package main

import (
	"examples/testpb"

	"github.com/chenxyzl/grain"
)

func main() {
	//warning: etcd url
	//create system
	system := grain.NewSystem("hello_reentry", "0.0.1", []string{"127.0.0.1:2379"})
	//start
	system.Start()
	//create b first
	helloActorB = system.Spawn(func() grain.IActor { return &HelloActorB{} })
	//create a then
	helloActorA = system.Spawn(func() grain.IActor { return &HelloActorA{} })
	//tell
	helloActorA.Send(&testpb.Hello{Name: "hello tell"})
	helloActorB.Send(&testpb.Hello{Name: "hello tell"})
	//waiting ctrl+c
	system.WaitStopSignal()
}
