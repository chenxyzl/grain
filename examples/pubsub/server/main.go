package main

import (
	"examples/pubsub/shared"
	"examples/testpb"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/chenxyzl/grain"
)

var _ grain.IActor = (*PlayerActor)(nil)

type PlayerActor struct {
	grain.BaseActor
	times int
}

func (p *PlayerActor) Started() {
	p.GetSystem().Subscribe(p.Self(), &testpb.Hello{})
	p.Logger().Info("Started")
}

func (p *PlayerActor) PreStop() {
	p.GetSystem().Unsubscribe(p.Self(), &testpb.Hello{})
	p.Logger().Info("PreStop")
}

func (p *PlayerActor) Receive(ctx grain.Context) {
	switch msg := ctx.Message().(type) {
	case *testpb.Hello:
		p.times++
		p.Logger().Info("reev publish msg", "name", msg.Name, "times", p.times)
	case *testpb.HelloAsk:
		ctx.Reply(&testpb.HelloReply{})
		p.Logger().Info("hello replay")
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", msg.ProtoReflect().Descriptor().FullName(), msg))
	}
}

func main() {
	grain.InitLog("./test.log", slog.LevelInfo)
	//new
	system := grain.NewSystem("pubsub_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		grain.WithConfigAskTimeout(time.Second*100),
		grain.WithConfigKind("player", func() grain.IActor { return &PlayerActor{} }))
	//start
	system.Logger().Warn("system starting")
	//
	system.Start()
	//
	system.Logger().Warn("system started successfully")
	// create a cluster actor
	_, err := grain.NoReentryAsk[*testpb.HelloReply](system.GetClusterActorRef("player", "cluster_player"), &testpb.HelloAsk{Name: "xxx"})
	if err != nil {
		panic(err)
	}
	//create a local actor
	system.SpawnNamed(func() grain.IActor { return &PlayerActor{} }, "local_player")

	times := 0
	for {
		time.Sleep(time.Second)
		system.PublishLocal(&testpb.Hello{Name: "local:zzzzzz:times:" + strconv.Itoa(times)}) //actor can recv
		system.Logger().Info("publish local", "times", times)
		if times++; times == shared.PublishTimes {
			break
		}
	}

	//run wait
	system.WaitStopSignal()
	//
	system.Logger().Warn("system stopped successfully")
}
