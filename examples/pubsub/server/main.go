package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"time"
)

var _ actor.IActor = (*PlayerActor)(nil)

type PlayerActor struct {
	actor.BaseActor
	times int
}

func (p *PlayerActor) Started() {
	p.System().Subscribe(p.Self(), &testpb.Hello{})
	p.Logger().Info("Started")
}

func (p *PlayerActor) PreStop() {
	p.System().Unsubscribe(p.Self(), &testpb.Hello{})
	p.Logger().Info("PreStop")
}

func (p *PlayerActor) Receive(ctx actor.IContext) {
	switch msg := ctx.Message().(type) {
	case *testpb.Hello:
		p.times++
		p.Logger().Info("reev publish msg", "name", msg.Name, "times", p.times)
		if p.times >= 3 {
			p.System().Unsubscribe(p.Self(), &testpb.Hello{})
		}
	case *testpb.HelloRequest:
		ctx.Reply(&testpb.HelloReply{})
		p.Logger().Info("hello replay")
	default:
		panic("not register msg type")
	}
}

func main() {
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("pubsub_cluster", "0.0.1", []string{"127.0.0.1:2379"}).WithKind("player", func() actor.IActor { return &PlayerActor{} })
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
