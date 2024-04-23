package main

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/singal"
)

func main() {
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"})
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Logger().Warn("system starting")
	if err := system.Start(); err != nil {
		panic(err)
	}
	system.Logger().Warn("system started successfully")
	//run wait
	singal.WaitStopSignal(system.Logger())
	//stop
	system.Logger().Warn("system stopping")
	system.Stop()
	system.Logger().Warn("system stopped successfully")
}
