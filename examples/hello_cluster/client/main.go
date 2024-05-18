package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"time"
)

func main() {
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"})
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config.WithRequestTimeout(time.Second * 100))
	//start
	system.Logger().Warn("system starting")
	//
	system.Start()
	//
	system.Logger().Warn("system started successfully")
	//
	remote := system.GetRemoteActorRef("player", "123456")
	system.Logger().Info("request target", "remote", remote)
	reply, err := actor.NoEntryRequestE[*testpb.HelloReply](system, remote, &testpb.HelloRequest{Name: "xxx"})
	if err != nil {
		panic(err)
	}
	system.Logger().Info("reply success", "name", reply.Name)

	//run wait
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
