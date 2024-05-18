package actor

import (
	"errors"
	"github.com/chenxyzl/grain/actor/uuid"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (x *System) Start() {
	//lock config
	x.config.markRunning()
	//register to cluster
	if err := x.clusterProvider.start(x, x.config); err != nil {
		panic(errors.Join(err, errors.New("cluster provider start failed")))
	}
}

func (x *System) running(nodeId uint64) {
	//update uuid node
	if err := uuid.Init(nodeId); err != nil {
		panic(errors.Join(err, errors.New("uuid init failed")))
	}
	x.requestId = uuid.GetBeginRequestId()
	x.Logger().Warn("uuid init success", "nodeId", nodeId)

	//overwrite logger
	x.logger = slog.With("system", x.clusterProvider.addr(), "node", x.config.state.NodeId)

	//init eventStream
	x.eventStream = x.SpawnNamed(func() IActor {
		return newEventStream(x.config.state.NodeId, x.clusterProvider.getEtcdClient(), x.clusterProvider.getEtcdLease(), x.config.GetEventStreamPrefix())
	}, eventStreamName, withKindName(defaultSystemKind))
}

func (x *System) WaitStopSignal() {
	// signal.Notify的ch信道是阻塞的(signal.Notify不会阻塞发送信号), 需要设置缓冲
	signals := make(chan os.Signal, 1)
	// It is not possible to block SIGKILL or syscall.SIGSTOP
	signal.Notify(signals, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	select {
	case sig := <-signals:
		x.Logger().Warn("system will exit by signal", "signal", sig.String())
	case <-x.forceCloseChan:
		x.Logger().Warn("system will exit by forceCloseChan")
	}
	x.stopActors()
	x.clusterProvider.stop()
}

func (x *System) ForceStop(err error) {
	x.logger.Error("system forceStop", "err", err)
	x.forceCloseChan <- true
}

func (x *System) stopActors() {
	x.Logger().Info("begin stop all actors")
	for {
		var left []*ActorRef
		times := 0
		x.registry.lookup.IterCb(func(key string, v iProcess) {
			if v.self().GetAddress() == x.clusterProvider.addr() &&
				v.self().GetKind() != defaultReplyKind {
				x.sendWithoutSender(v.self(), messageDef.poison)
				left = append(left, v.self())
			}
		})
		time.Sleep(time.Second)
		times++
		if len(left) == 0 {
			x.Logger().Warn("waiting actors stop success", "times", times)
			break
		} else if times >= defaultStopWaitTimeSecond {
			x.Logger().Warn("waiting stop timeout", "left", len(left), "times", times, "actors", left)
			break
		} else {
			x.Logger().Info("waiting actors stop ..., ", "count", len(left), "times", times)
		}
	}
}
