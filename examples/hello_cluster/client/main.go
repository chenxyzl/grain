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
	//new system
	system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigRequestTimeout(time.Second*1))
	//start
	system.Logger().Warn("system starting")
	system.Start()
	system.Logger().Warn("system started successfully")
	//get a cluster actorRef
	actorRef := system.GetClusterActorRef("player", "123456")
	if actorRef == nil {
		panic("GetClusterActorRef failed")
	}
	//
	go func() {
		c := time.NewTicker(3 * time.Second)
		times := 0
		for range c.C {
			times++
			//tell
			actorRef.Send(&testpb.Hello{Name: "hello tell, times:" + strconv.Itoa(times)})
			//request
			system.Logger().Info("request: ", "target", actorRef)
			reply, err := grain.NoReentryRequest[*testpb.HelloReply](actorRef, &testpb.HelloRequest{Name: "xxx, times:" + strconv.Itoa(times)})
			if err != nil {
				system.Logger().Error(err.Error())
			}
			system.Logger().Info("reply:", "message", reply)
		}
	}()

	//wait ctrl+c
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
