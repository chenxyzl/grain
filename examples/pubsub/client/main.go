package main

import (
	"examples/pubsub/shared"
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"strconv"
	"time"
)

func main() {
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("pubsub_cluster", "0.0.1", []string{"127.0.0.1:2379"})
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config.WithRequestTimeout(time.Second * 100))
	//start
	system.Logger().Warn("system starting")
	//
	system.Start()
	//
	system.Logger().Warn("system started successfully")

	//
	times := 0
	for {
		time.Sleep(time.Second)
		system.PublishGlobal(&testpb.Hello{Name: "global:xxxxx:times:" + strconv.Itoa(times)}) //actor can recv
		system.PublishLocal(&testpb.Hello{Name: "global:yyyyy:times:" + strconv.Itoa(times)})  //actor can't recv
		system.Logger().Info("publish global", "times", times)
		if times++; times == shared.PublishTimes {
			break
		}
	}

	//run wait
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
