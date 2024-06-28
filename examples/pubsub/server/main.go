package main

import (
	"examples/pubsub/shared"
	"examples/testpb"
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"log/slog"
	"strconv"
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

func (p *PlayerActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *testpb.Hello:
		p.times++
		p.Logger().Info("reev publish msg", "name", msg.Name, "times", p.times)
	case *testpb.HelloRequest:
		ctx.Reply(&testpb.HelloReply{})
		p.Logger().Info("hello replay")
	default:
		panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", msg.ProtoReflect().Descriptor().FullName(), msg))
	}
}

func main() {
	actor.InitLog("./test.log", slog.LevelInfo)
	//config
	config := actor.NewConfig("pubsub_cluster", "0.0.1", []string{"127.0.0.1:2379"},
		actor.WithConfigRequestTimeout(time.Second*100),
		actor.WithConfigKind("player", func() actor.IActor { return &PlayerActor{} }))
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Logger().Warn("system starting")
	//
	system.Start()
	//
	system.Logger().Warn("system started successfully")
	// create a remote actor
	_, err := actor.NoEntryRequestE[*testpb.HelloReply](system, system.GetRemoteActorRef("player", "12345"), &testpb.HelloRequest{Name: "xxx"})
	if err != nil {
		panic(err)
	}
	//create a local actor
	system.SpawnNamed(func() actor.IActor { return &PlayerActor{} }, "local_yyy")

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
