package grain

import (
	"fmt"
	"slices"
	"time"
)

const (
	// defaultMailboxInitSize is the starting mailbox capacity. The mailbox grows
	// (doubling) on demand up to defaultMailboxMaxSize, so this is a floor, not a limit.
	//
	// Deliberately small. The previous value of 128 was justified on throughput grounds
	// ("captures nearly all of the steady-state send throughput"), which was true of the
	// pre-v1.2.2 FIXED-capacity blocking mailbox but is not true of a growable one:
	// measured across init = 1, 4, 8, 16, 32, 128 and 512, local Tell throughput is
	// identical within noise (204-223 ns/op, 0% overflow at every size), because the ring
	// simply doubles on demand and steady-state queue depth is 0-1 anyway.
	//
	// What the old default did buy was 2304 bytes eagerly allocated and zeroed per actor
	// (~730ns of the ~4.2us spawn). At 8 slots that is 128 bytes — 2.2KB saved per actor,
	// i.e. ~220MB at 100k actors. The cost is paid only by actors that genuinely queue
	// deeply: growing 8 -> 128 costs ~1us and 4 extra allocations, once, and only for
	// those. A mailbox is a burst buffer, so most actors never get there.
	//
	// Use WithOptsMailboxSize to pre-reserve when a kind is known to burst.
	defaultMailboxInitSize = 8
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
