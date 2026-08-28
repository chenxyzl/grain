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
			// defaultRegisterTimes attempts with 0/100/200ms backoff. The backoff sits
			// BETWEEN attempts only: the old loop slept before checking the attempt
			// limit, so a run that was going to fail still burned a final 400ms — with
			// the actor's Started() blocked on it the whole time.
			for i := 0; i < defaultRegisterTimes; i++ {
				if i > 0 {
					time.Sleep(time.Millisecond * 100 * (1 << (i - 1)))
				}
				if clusterProvider.setTxn(config.getActorRegisterName(ref), ref.GetDirectAddr()) {
					return nil
				}
			}
			return fmt.Errorf("failed register cluster actor to clusterProvider, ref:%v", ref.GetId())
		}
		return nil
	}
	defaultUnregisterFromCluster = func(clusterProvider iProvider, config *config, ref ActorRef) error {
		//unRegister from cluster
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			// Returning the failure instead of swallowing it (the old body was an empty
			// `if removeTxn(...) {}`): a failed de-registration leaves a stale cluster
			// routing entry pointing at an actor that no longer exists, so peers keep
			// sending to it. The caller logs it.
			if !clusterProvider.removeTxn(config.getActorRegisterName(ref), ref.GetDirectAddr()) {
				return fmt.Errorf("failed unregister cluster actor from clusterProvider, ref:%v", ref.GetId())
			}
		}
		return nil
	}
)

type KindOptFunc func(*tOpts)

type tOpts struct {
	producer          Producer
	mailboxInitSize   int
	mailboxMaxSize    int
	kind              string
	poisonFirstOnQuit bool
	_self             ActorRef

	registerToCluster     func(clusterProvider iProvider, config *config, ref ActorRef) error
	unRegisterFromCluster func(clusterProvider iProvider, config *config, ref ActorRef) error
}

// newOpts ...
func newOpts(p Producer, opts ...KindOptFunc) tOpts {
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
