package main

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/singal"
)

func main() {
	//config
	config := actor.NewConfig("hello", []string{"127.0.0.1:2379"})
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	if err := system.Start(); err != nil {
		panic(err)
	}
	//run wait
	singal.WaitStopSignal(system.Logger())
	//stop
	system.Stop()
}
