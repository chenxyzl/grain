package main

import (
	"github.com/chenxyzl/grain/actor"
)

var _ actor.IActor = (*PlayerActor)(nil)

type PlayerActor struct {
	actor.BaseActor
}

func (p *PlayerActor) Started() {
	p.Logger().Info("Started")
}

func (p *PlayerActor) PreStop() {
	p.Logger().Info("PreStop")
}

func (p *PlayerActor) Receive(ctx actor.IContext) {
	p.Logger().Info(string(ctx.Message().ProtoReflect().Descriptor().FullName()))
}

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
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
