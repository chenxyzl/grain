package grain

import (
	"fmt"
	"slices"
	"time"
)

const (
	// defaultMailboxInitSize is the starting mailbox capacity. The mailbox grows
	// (doubling) on demand up to defaultMailboxMaxSize, so idle actors stay small.
	// 128 is the throughput/memory sweet spot: it captures nearly all of the
	// steady-state send throughput while keeping an idle actor at ~2KB.
	defaultMailboxInitSize = 128
	// defaultMailboxMaxSize is the hard ceiling; once reached, further messages
	// overflow to a dead letter instead of blocking the sender.
	defaultMailboxMaxSize = 4096
	defaultRegisterTimes  = 3
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
	mailboxInitSize   int
	mailboxMaxSize    int
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
		mailboxInitSize:       defaultMailboxInitSize,
		mailboxMaxSize:        defaultMailboxMaxSize,
		poisonFirstOnQuit:     true,
		kind:                  defaultLocalKind,
		registerToCluster:     defaultRegisterToCluster,
		unRegisterFromCluster: defaultUnregisterFromCluster,
	}
	for _, opt := range opts {
		opt(&ret)
	}
	// clamp: init >= 1, max >= init
	if ret.mailboxInitSize < 1 {
		ret.mailboxInitSize = 1
	}
	if ret.mailboxMaxSize < ret.mailboxInitSize {
		ret.mailboxMaxSize = ret.mailboxInitSize
	}
	return ret
}
