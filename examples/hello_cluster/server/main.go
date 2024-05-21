package main

import (
	"examples/share_actor"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
)

func main() {
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		actor.WithConfigKind("player", func() actor.IActor { return &share_actor.HelloActor{} }))
	//system
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Logger().Warn("system starting")
	system.Start()
	system.Logger().Warn("system started successfully")
	//wait ctrl+c
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
