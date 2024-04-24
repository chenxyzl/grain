package main

import (
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
)

func main() {
	//log
	helper.InitLog("./test.log")
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
	system.WaitStopSignal()
	//stop
	system.Logger().Warn("system stopped")
}
