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
	// A Serve failure means this node can no longer receive anything, so escalate it into the
	// normal shutdown path instead of letting the transport panic the process from a goroutine.
	x.rpcService = remote.NewRpcServer(x.RecvEnvelope, x.config.grpcListenAddr, x.ForceStop)
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
	drained := x.stopActors()
	if x.clusterProvider != nil {
		// Only give up the lease if every actor really stopped; see stopActors.
		x.clusterProvider.stop(drained)
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

// stopActors drains every actor, in two phases: those that opted into being poisoned first, then
// the rest. It reports whether they ALL stopped.
//
// That result is load-bearing, not informational. Every grain's single-activation lock hangs off
// this node's etcd lease, so revoking the lease while an activation is still running frees its
// lock — a peer then activates the same grain and the two write the same state (lost update), no
// network partition needed. A member key left behind for one lease TTL is the strictly lesser
// evil, so a false result must suppress the revoke.
func (x *system) stopActors() bool {
	x.Logger().Info("stop all actors begin")
	// Refuse on-demand activation from here on: grpc still listens (stopped only after this
	// returns), so inbound envelopes would spawn actors nothing poisons.
	x.draining.Store(true)
	// wake Asks blocked on a reply so shutdown doesn't wait out askTimeout
	x.wakePendingAsks()
	// both phases run even if the first times out: the second still poisons what is left
	drainedFirst := x.stopActorsImpl(true)
	drainedLatter := x.stopActorsImpl(false)
	if drainedFirst && drainedLatter {
		x.Logger().Info("stop all actors success")
		return true
	}
	x.Logger().Error("stop all actors INCOMPLETE: actors are still running, so this node will " +
		"NOT revoke its etcd lease; their grain locks stay held until it expires, which keeps " +
		"peers from double-activating them")
	return false
}

// wakePendingAsks closes registration before taking the snapshot. A racing Ask
// either completes insertion before the snapshot or is refused. Reply/cancel may
// remove entries concurrently under the map locks; reply delivery and wakeup are
// non-blocking, and canceled callers no longer need a wakeup. Lock order is always
// admission lock then map shard lock; no map callback acquires admission.
func (x *system) wakePendingAsks() {
	x.pendingRegistrationMu.Lock()
	x.pendingRegistrationClosed = true
	var replies []chan proto.Message
	x.pending.IterCb(func(_ uint64, reply chan proto.Message) { replies = append(replies, reply) })
	x.pendingRegistrationMu.Unlock()
	for _, reply := range replies {
		select {
		case reply <- errSystemStopping:
		default:
		}
	}
}

// stopPollInterval is how often the drain loop re-checks the registry. Well under a second, so a
// healthy shutdown costs one interval rather than a full second per phase.
const stopPollInterval = 50 * time.Millisecond

// stopActorsImpl poisons every actor belonging to this phase and waits for them to leave the
// registry, up to stopWaitTimeSecond. Reports whether the phase fully drained.
func (x *system) stopActorsImpl(first bool) bool {
	phase := "<<latter>> "
	if first {
		phase = "<<first>> "
	}
	x.Logger().Info(phase + "stop actors begin")
	deadline := time.Now().Add(time.Duration(x.getConfig().stopWaitTimeSecond) * time.Second)
	// one "still waiting" line per second, not per 50ms poll
	const logEvery = int(time.Second / stopPollInterval)
	for round := 1; ; round++ {
		var left []ActorRef
		// rangeIt, not lookup.IterCb: poison() wakes a run loop that can reach registry.remove,
		// and IterCb holds a shard read lock for the whole callback (see registry.rangeIt).
		x.registry.rangeIt(func(_ string, v iProcess) {
			// the first phase only claims actors that opted into being poisoned early
			if first && v.opts() != nil && !v.opts().poisonFirstOnQuit {
				return
			}
			v.poison() // idempotent: sets the flag and wakes the run loop
			left = append(left, v.self())
		})
		if len(left) == 0 {
			x.Logger().Info(phase+"stop actors success", "rounds", round)
			return true
		}
		if !time.Now().Before(deadline) {
			x.Logger().Error(phase+"stop actors TIMED OUT, actors are still running",
				"left", len(left), "rounds", round,
				"stopWaitTimeSecond", x.getConfig().stopWaitTimeSecond, "actors", left)
			return false
		}
		if round%logEvery == 1 {
			x.Logger().Info(phase+"waiting actors stop", "count", len(left), "round", round)
		}
		time.Sleep(stopPollInterval)
	}
}
