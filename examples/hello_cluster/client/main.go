package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"log/slog"
	"time"
)

func main() {
	actor.InitLog("./test.log", slog.LevelInfo)
	//config
	config := actor.NewConfig("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		actor.WithConfigRequestTimeout(time.Second*1))
	//new system
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Logger().Warn("system starting")
	system.Start()
	system.Logger().Warn("system started successfully")
	//get a remote actorRef
	actorRef := system.GetRemoteActorRef("player", "123456")
	//tell
	actor.NoReentrySend(system, actorRef, &testpb.Hello{Name: "hello tell"})
	//request
	system.Logger().Info("request: ", "target", actorRef)
	reply, err := actor.NoReentryRequest[*testpb.HelloReply](system, actorRef, &testpb.HelloRequest{Name: "xxx"})
	if err != nil {
		panic(err)
	}
	system.Logger().Info("reply:", "message", reply)

	//wait ctrl+c
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
