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
	"google.golang.org/protobuf/proto"
)

func (x *system) Start() {
	//start grpc_server
	x.rpcService = remote.NewRpcServer(x.RecvEnvelope)
	//start grpc
	if err := x.rpcService.Start(); err != nil {
		panic(errors.Join(err, errors.New("grpc server start failed")))
	}
	//cache the (now-fixed) listen address so getAddr() is a field read
	x.addr = x.rpcService.Addr()
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
	// wake any Ask blocked waiting for a reply, so shutdown doesn't wait out
	// askTimeout. Delivers the poison sentinel to each pending channel, which
	// awaitReply maps to a "poisoned" error.
	x.wakePendingAsks()
	x.stopActorsImpl(true)
	x.stopActorsImpl(false)
	x.Logger().Info("stop all actors success")
}

// wakePendingAsks delivers the poison sentinel to every waiting Ask so shutdown
// doesn't wait out askTimeout. Safe against a concurrent deliverReply on the
// same snId: the send here is non-blocking (select/default) and the channel is
// cap-1, so if a real reply already filled the buffer this poison is simply
// dropped; awaitReply returns on whichever value it receives. IterCb holds the
// shard RLock while deliverReply's Pop takes the write lock, so per snId they
// are serialized rather than truly simultaneous.
func (x *system) wakePendingAsks() {
	x.pending.IterCb(func(_ uint64, ch chan proto.Message) {
		select {
		case ch <- poison:
		default:
		}
	})
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
