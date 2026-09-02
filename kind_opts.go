package grain

import (
	"fmt"
	"slices"
	"time"
)

const (
	// defaultMailboxInitSize is the starting mailbox capacity; the mailbox doubles on demand up
	// to defaultMailboxMaxSize, so this is a floor, not a limit. Deliberately small: local Tell
	// throughput is identical from 1 to 512 slots (the ring just grows, steady-state depth is
	// 0-1), while 128 slots cost 2304 bytes eagerly zeroed per actor. Use WithOptsMailboxSize to
	// pre-reserve for a kind known to burst.
	defaultMailboxInitSize = 8
	// defaultMailboxMaxSize is the hard ceiling; once reached, further messages
	// overflow to a dead letter instead of blocking the sender.
	defaultMailboxMaxSize = 4096
	defaultRegisterTimes  = 3
)

// The grain registration key is the cluster's single-activation lock: setTxn is create-only, so
// of two nodes racing to activate the same cluster actor exactly one wins and the loser's start()
// stops it before Started() ever runs. The VALUE is the owning node's address, and it is
// load-bearing rather than informational: removeTxn is a compare-and-delete against it, which is
// what makes "delete only the lock I hold" true. A value that is not per-node degenerates that
// into an unconditional delete, letting the loser free the winner's lock and the grain run twice.
var (
	defaultRegisterToCluster = func(clusterProvider iProvider, config *config, ref ActorRef) error {
		//register to cluster
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			// defaultRegisterTimes attempts with 0/100/200ms backoff. The sleep sits BETWEEN
			// attempts only, so a doomed run does not burn a final wait with Started() blocked.
			for i := 0; i < defaultRegisterTimes; i++ {
				if i > 0 {
					time.Sleep(time.Millisecond * 100 * (1 << (i - 1)))
				}
				if clusterProvider.setTxn(config.getActorRegisterName(ref), config.state.Address) {
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
			// Reported, not swallowed: a failed de-registration leaves a stale routing entry
			// pointing at an actor that no longer exists, so peers keep sending to it.
			if !clusterProvider.removeTxn(config.getActorRegisterName(ref), config.state.Address) {
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
