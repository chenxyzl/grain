package main

import (
	"examples/pubsub/shared"
	"examples/testpb"
	"log/slog"
	"strconv"
	"time"

	"github.com/chenxyzl/grain"
)

func main() {
	grain.InitLog("./test.log", slog.LevelInfo)
	//new
	system := grain.NewSystem("pubsub_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigAskTimeout(time.Second*100))
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
