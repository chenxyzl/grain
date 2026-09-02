package main

import (
	"examples/testpb"
	"log/slog"
	"strconv"
	"time"

	"github.com/chenxyzl/grain"
)

func main() {
	grain.InitLog("./test.log", slog.LevelInfo)
	system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigAskTimeout(time.Second*1))
	system.Logger().Warn("system starting")
	system.Start()
	system.Logger().Warn("system started successfully")
	actorRef := system.GetClusterActorRef("player", "123456")
	if actorRef == nil {
		panic("GetClusterActorRef failed")
	}

	c := time.NewTicker(3 * time.Second)
	go func() {
		times := 0
		for range c.C {
			times++
			actorRef.Tell(&testpb.Hello{Name: "hello tell, times:" + strconv.Itoa(times)})
			system.Logger().Info("ask: ", "target", actorRef)
			reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, &testpb.HelloAsk{Name: "xxx, times:" + strconv.Itoa(times)})
			if err != nil {
				system.Logger().Error(err.Error())
			}
			system.Logger().Info("reply:", "message", reply)
		}
	}()

	//wait ctrl+c
	system.WaitStopSignal(func() { c.Stop() }, nil)
	system.Logger().Warn("system stopped successfully")
}
