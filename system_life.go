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
	x.askId = uuid.GetAskStartId()
	x.Logger().Warn("uuid init success", "nodeId", nodeId)

	//overwrite logger
	x.logger = slog.With("system", x.getAddr(), "node", x.config.state.NodeId)

	//init eventStream
	x.eventStream = x.SpawnNamed(func() IActor {
		return newEventStream(x.config.state.NodeId, x.clusterProvider.getEtcdClient(), x.clusterProvider.getEtcdLease(), x.config.getEventStreamWatchPath())
	}, eventStreamWatchName, WithOptsKindName(defaultSystemKind), WithOptsPoisonFirstOnQuit(false))
}

func (x *system) WaitStopSignal(beforeQuit func(), afterQuit func()) {
	// signal.Notify的ch信道是阻塞的(signal.Notify不会阻塞发送信号), 需要设置缓冲
	signals := make(chan os.Signal, 1)
	// It is not possible to block SIGKILL or syscall.SIGSTOP
	signal.Notify(signals, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	select {
	case sig := <-signals:
		x.Logger().Warn("system will exit by signal", "signal", sig.String())
		//
		if beforeQuit != nil {
			beforeQuit()
		}
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
	//
	if afterQuit != nil {
		afterQuit()
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
	x.Logger().Info("stop all actors begin")
	x.stopActorsImpl(true)
	x.stopActorsImpl(false)
	x.Logger().Info("stop all actors success")
}

func (x *system) stopActorsImpl(first bool) {
	firstStr := "<<first>>"
	if !first {
		firstStr = "<<latter>>"
	}
	x.Logger().Info(firstStr + "stop actors begin")
	times := 0
	for {
		//interval wait
		if times != 0 {
			time.Sleep(time.Second)
		}
		//check left actors
		var left []ActorRef
		x.registry.lookup.IterCb(func(key string, v iProcess) {
			//reply processors are transient RPC-reply holders: poison them to wake
			//any caller blocked in Ask/Result, but don't count them in `left` —
			//they remove themselves on Result() and must not stall shutdown.
			if v.self().isAsk() {
				v.poison()
				return
			}
			//posion sequence
			if first { //first poison
				if v.opts() != nil && !v.opts().poisonFirstOnQuit {
					return
				}
			} else { //later poison

			}
			//
			v.poison()
			left = append(left, v.self())
		})
		times++
		if len(left) == 0 {
			x.Logger().Warn(firstStr+"waiting actors stop success", "times", times)
			break
		} else if times >= x.getConfig().stopWaitTimeSecond {
			x.Logger().Warn(firstStr+"waiting stop timeout", "left", len(left), "times", times, "actors", left)
			break
		} else {
			x.Logger().Info(firstStr+"waiting actors stop", "count", len(left), "times", times)
		}
	}
	x.Logger().Info(firstStr + "stop actors success")
}
