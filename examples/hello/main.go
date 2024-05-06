package main

import (
	"examples/testpb"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/utils/helper"
	"log/slog"
	"time"
)

type HelloGoActor struct {
	actor.BaseActor
}

func (x *HelloGoActor) Started() error {
	x.Logger().Info("Started")
	return nil
}
func (x *HelloGoActor) PreStop() error {
	x.Logger().Info("PreStop")
	return nil
}
func (x *HelloGoActor) Receive(context actor.IContext) {
	switch msg := context.Message().(type) {
	case *testpb.Hello:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("tell: %v", msg.GetName()))
		}
	case *testpb.HelloRequest:
		{
			_ = msg
			//x.Logger().Info(fmt.Sprintf("request: %v", msg.GetName()))
			if context.Sender() != nil {
				x.System().Send(context.Sender(), &testpb.HelloReply{Name: "hell go reply"})
			}
		}
	default:
		x.Logger().Error("xxx")
	}
}

func main() {
	//log
	helper.InitLog("./test.log")
	//config
	config := actor.NewConfig("hello", "0.0.1", []string{"127.0.0.1:2379"}).WithRequestTimeout(time.Second)
	//new
	system := actor.NewSystem[*actor.ProviderEtcd](config)
	//start
	system.Logger().Warn("system starting")
	if err := system.Start(); err != nil {
		panic(err)
	}
	system.Logger().Warn("system started successfully")

	actorRef := system.Spawn(func() actor.IActor { return &HelloGoActor{} })
	system.Send(actorRef, &testpb.Hello{Name: "helle grain"})

	reply := actor.Request[*testpb.HelloReply](system, actorRef, &testpb.HelloRequest{Name: "hello go request"})
	slog.Info(reply.String())

	//run wait
	system.WaitStopSignal()
	//stop
	system.Logger().Warn("system stopped")
}
