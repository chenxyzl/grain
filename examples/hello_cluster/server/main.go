package main

import (
	"examples/share_actor"
	"log/slog"

	"github.com/chenxyzl/grain"
)

func main() {
	grain.InitLog("./test.log", slog.LevelInfo)
	//system
	system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigKind("player", func() grain.IActor { return &share_actor.HelloActor{} }))
	//start
	system.Logger().Warn("system starting")
	system.Start()
	system.Logger().Warn("system started successfully")
	//wait ctrl+c
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
