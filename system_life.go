package grain

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chenxyzl/grain/remote"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
)

func (x *system) Start() {
	x.rpcService = remote.NewRpcServer(x.RecvEnvelope, x.config.grpcListenAddr)
	if err := x.rpcService.Start(); err != nil {
		panic(errors.Join(err, errors.New("grpc server start failed")))
	}
	//now fixed: cache it so getAddr() is a field read
	x.addr = x.rpcService.Addr()
	x.logger.Store(x.Logger().With("system", x.rpcService.Addr()))
	if err := x.clusterProvider.start(x, x.clusterMemberChanged, x.getAddr(), x.config, x.Logger()); err != nil {
		panic(errors.Join(err, errors.New("cluster provider start failed")))
	}
}

func (x *system) init(nodeId uint64) {
	x.config.markRunning()
	if err := uuid.Init(nodeId); err != nil {
		panic(errors.Join(err, errors.New("uuid init failed")))
	}
	x.askId = uuid.GetAskStartId()
	x.Logger().Warn("uuid init success", "nodeId", nodeId)

	//init() is only reached from Start(), which already tagged "system", so only add "node"
	x.logger.Store(x.Logger().With("node", x.config.state.NodeId))

	eventStreamRef, err := x.SpawnNamed(func() IActor {
		return newEventStream(x.config.state.NodeId, x.clusterProvider, x.config.getEventStreamWatchPath())
	}, eventStreamWatchName, WithOptsKindName(defaultSystemKind), WithOptsPoisonFirstOnQuit(false))
	if err != nil {
		// init runs once per system, so the fixed eventStream name cannot be taken already
		panic(errors.Join(err, errors.New("event stream spawn failed")))
	}
	x.eventStream = eventStreamRef
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
	// Order matters: drain actors, then leave the cluster, then stop grpc — inbound envelopes
	// must stay routable while actors are still being poisoned.
	x.stopActors()
	if x.clusterProvider != nil {
		x.clusterProvider.stop()
	}
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
		x.Logger().Error("system forceStop", "err", err)
	} else {
		x.Logger().Warn("system forceStop")
	}
	// Non-blocking: cap-1 chan drained only by WaitStopSignal, so a plain send would park the
	// caller (etcd keepalive/watch goroutines) forever on a second stop request.
	select {
	case x.forceCloseChan <- true:
	default:
		x.Logger().Warn("system forceStop already requested, ignoring")
	}
}

func (x *system) stopActors() {
	x.Logger().Info("stop all actors begin")
	// Refuse on-demand activation from here on: grpc still listens (stopped only after this
	// returns), so inbound envelopes would spawn actors nothing poisons.
	x.draining.Store(true)
	// wake Asks blocked on a reply so shutdown doesn't wait out askTimeout
	x.wakePendingAsks()
	x.stopActorsImpl(true)
	x.stopActorsImpl(false)
	x.Logger().Info("stop all actors success")
}

// wakePendingAsks poisons every waiting Ask so shutdown doesn't wait out askTimeout. The send
// is non-blocking into a cap-1 chan, so it loses harmlessly to a concurrent real reply.
func (x *system) wakePendingAsks() {
	x.pending.IterCb(func(_ uint64, ch chan proto.Message) {
		select {
		case ch <- msgPoison:
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
		if times != 0 {
			time.Sleep(time.Second)
		}
		var left []ActorRef
		x.registry.lookup.IterCb(func(key string, v iProcess) {
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
