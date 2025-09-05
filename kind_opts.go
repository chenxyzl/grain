package grain

import (
	"context"
	"math"
	"slices"
	"time"
)

const (
	defaultMailboxSize = 1024
	defaultMaxRestarts = 3
)

var (
	defaultRestartDelay = func(restartTimes int) time.Duration {
		if restartTimes < 1 {
			restartTimes = 1
		}
		return 100 * time.Millisecond * time.Duration(math.Pow(2, float64(restartTimes-1)))
	}
	defaultRegisterToCluster = func(clusterProvider iProvider, config *config, ref ActorRef) {
		//register to cluster
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			times := 0
			registerSuccess := false
			for {
				times++
				if times >= 2 {
					time.Sleep(time.Millisecond * 100 * (1 << (times - 2)))
				}
				if times > defaultRegisterTimes {
					break
				}
				if !clusterProvider.setTxn(config.getActorRegisterName(ref), ref.GetDirectAddr()) {
					continue
				}
				//
				registerSuccess = true
				break
			}
			if !registerSuccess {
				panic("failed register cluster actor to clusterProvider")
			}
		}
	}
	defaultUnregisterFromCluster = func(clusterProvider iProvider, config *config, ref ActorRef) {
		//unRegister from cluster
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			if clusterProvider.removeTxn(config.getActorRegisterName(ref), ref.GetDirectAddr()) {
			}
		}
	}
)

type KindOptFunc func(*tOpts)

type tOpts struct {
	producer     iProducer
	mailboxSize  int
	kind         string
	maxRestarts  int32
	restartDelay func(restartTimes int) time.Duration
	context      context.Context
	_self        ActorRef

	registerToCluster     func(clusterProvider iProvider, config *config, ref ActorRef)
	unRegisterFromCluster func(clusterProvider iProvider, config *config, ref ActorRef)
}

// newOpts ...
func newOpts(p iProducer, opts ...KindOptFunc) tOpts {
	ret := tOpts{
		producer:              p,
		mailboxSize:           defaultMailboxSize,
		kind:                  defaultLocalKind,
		maxRestarts:           defaultMaxRestarts,
		restartDelay:          defaultRestartDelay,
		context:               context.Background(),
		registerToCluster:     defaultRegisterToCluster,
		unRegisterFromCluster: defaultUnregisterFromCluster,
	}
	for _, opt := range opts {
		opt(&ret)
	}
	return ret
}
