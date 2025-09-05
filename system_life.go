package grain

import (
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chenxyzl/grain/remote"
	"github.com/chenxyzl/grain/uuid"
)

func (x *system) Start() {
	//start grpc_server
	x.rpcService = remote.NewRpcServer(x.RecvEnvelope)
	//start grpc
	if err := x.rpcService.Start(); err != nil {
		panic(errors.Join(err, errors.New("grpc server start failed")))
	}
	//init logger
	x.logger = slog.With("system", x.rpcService.Addr())
	//register to cluster
	if err := x.clusterProvider.start(x, x.clusterMemberChanged, x.getAddr(), x.config, x.logger); err != nil {
		panic(errors.Join(err, errors.New("cluster provider start failed")))
	}
}

func (x *system) init(nodeId uint64) {
	//lock config
	x.config.markRunning()
	//update uuid node
	if err := uuid.Init(nodeId); err != nil {
		panic(errors.Join(err, errors.New("uuid init failed")))
	}
	x.requestId = uuid.GetBeginRequestId()
	x.Logger().Warn("uuid init success", "nodeId", nodeId)

	//overwrite logger
	x.logger = slog.With("system", x.getAddr(), "node", x.config.state.NodeId)

	//init eventStream
	x.eventStream = x.SpawnNamed(func() IActor {
		return newEventStream(x.config.state.NodeId, x.clusterProvider.getEtcdClient(), x.clusterProvider.getEtcdLease(), x.config.getEventStreamWatchPath())
	}, eventStreamWatchName, WithOptsKindName(defaultSystemKind))
}

func (x *system) WaitStopSignal() {
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
	//stop actors
	x.stopActors()
	//stop clusterProvider
	if x.clusterProvider != nil {
		x.clusterProvider.stop()
	}
	//stop grpc
	if x.rpcService != nil {
		err := x.rpcService.Stop()
		if err != nil {
			x.Logger().Warn("rpc service stop err", "err", err)
		}
	}
}

func (x *system) ForceStop(err error) {
	if err != nil {
		x.logger.Error("system forceStop", "err", err)
	} else {
		x.logger.Warn("system forceStop")
	}
	x.forceCloseChan <- true
}

func (x *system) stopActors() {
	x.Logger().Info("begin stop all actors")
	for {
		var left []ActorRef
		times := 0
		x.registry.lookup.IterCb(func(key string, v iProcess) {
			//ignore poison
			if v.self().GetDirectAddr() != x.getAddr() ||
				v.self().GetKind() == defaultReplyKind ||
				v.self().GetKind() == defaultSystemKind {
				return
			}
			//
			x.tell(v.self(), poison)
			left = append(left, v.self())
		})
		time.Sleep(time.Second)
		times++
		if len(left) == 0 {
			x.Logger().Warn("waiting actors stop success", "times", times)
			break
		} else if times >= x.getConfig().stopWaitTimeSecond {
			x.Logger().Warn("waiting stop timeout", "left", len(left), "times", times, "actors", left)
			break
		} else {
			x.Logger().Info("waiting actors stop ..., ", "count", len(left), "times", times)
		}
	}
}
