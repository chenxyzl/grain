package grain

import (
	"fmt"
	"slices"
	"time"
)

const (
	defaultMailboxSize   = 1024
	defaultRegisterTimes = 3
)

var (
	defaultRegisterToCluster = func(clusterProvider iProvider, config *config, ref ActorRef) error {
		//register to cluster
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			times := 0
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
				return nil
			}
			return fmt.Errorf("failed register cluster actor to clusterProvider, ref:%v", ref.GetId())
		}
		return nil
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
	producer          iProducer
	mailboxSize       int
	kind              string
	poisonFirstOnQuit bool
	_self             ActorRef

	registerToCluster     func(clusterProvider iProvider, config *config, ref ActorRef) error
	unRegisterFromCluster func(clusterProvider iProvider, config *config, ref ActorRef)
}

// newOpts ...
func newOpts(p iProducer, opts ...KindOptFunc) tOpts {
	ret := tOpts{
		producer:              p,
		mailboxSize:           defaultMailboxSize,
		poisonFirstOnQuit:     true,
		kind:                  defaultLocalKind,
		registerToCluster:     defaultRegisterToCluster,
		unRegisterFromCluster: defaultUnregisterFromCluster,
	}
	for _, opt := range opts {
		opt(&ret)
	}
	return ret
}
