package main

import (
	"examples/testpb"
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"time"
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
	switch msg := ctx.Message().(type) {
	case *testpb.HelloRequest:
		ctx.Reply(&testpb.HelloReply{Name: msg.GetName()})
		p.Logger().Info("recv request hello, reply", "name", msg.Name)
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", msg.ProtoReflect().Descriptor().FullName(), msg))
	}
}

func main() {
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"}).WithKind("player", func() actor.IActor { return &PlayerActor{} })
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config.WithRequestTimeout(time.Second * 100))
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
