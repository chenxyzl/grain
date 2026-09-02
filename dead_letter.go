package grain

import (
	"github.com/chenxyzl/grain/ghelper"
	"google.golang.org/protobuf/proto"
)

// Dead-letter reasons.
const (
	DeadLetterReasonOverflow = "mailbox overflow" // mailbox full and at max capacity
	DeadLetterReasonStopped  = "actor stopped"    // target actor already stopped
	// DeadLetterReasonNotFound: nothing was registered to deliver to on the node that owns the
	// target — a local actor that already stopped, or a cluster grain that could not be activated.
	DeadLetterReasonNotFound = "actor not found"
	// DeadLetterReasonNoOwner: the target is a cluster ref whose kind no node in the cluster
	// hosts, so there is nowhere to route it. Usually a missing WithConfigKind.
	DeadLetterReasonNoOwner = "kind not hosted by any node"
	// DeadLetterReasonUnreachable: the outbound stream to the target's node is not open, so the
	// message cannot leave this node.
	DeadLetterReasonUnreachable = "remote node unreachable"
)

// DeadLetter describes an undeliverable message, surfaced to the DeadLetterHandler set via
// WithConfigDeadLetter (defaults to a WARN log).
type DeadLetter struct {
	Target ActorRef // intended recipient; for a cross-node send the REMOTE actor
	// Owner is the actor whose mailbox actually rejected it — on the outbound path the local
	// write_stream actor, not Target, meaning the outbound stream is the bottleneck. Nil when no
	// mailbox was involved at all, i.e. nothing was found to deliver to.
	Owner   ActorRef
	Sender  ActorRef // may be nil
	Message proto.Message
	MsgSnId uint64
	Reason  string
}

// DeadLetterHandler receives undeliverable messages. Invoked on the SENDER's goroutine, so it
// must be fast and non-blocking. A panic in it is recovered and logged, not propagated.
type DeadLetterHandler func(DeadLetter)

// emitDeadLetter hands dl to the configured DeadLetterHandler, or logs WARN when none is set. Runs
// on the sender's goroutine, so a panicking handler is recovered here rather than taking down
// whatever was doing the send.
//
// This is the single funnel for undeliverable messages: every drop the framework knows about goes
// through it, so a monitoring hook sees all of them. It deliberately does NOT fail a waiting Ask —
// callers reply the specific error that fits their case (errActorNotFound, errKindNotInCluster,
// ...), which tells the asker more than a generic reason string would.
func (x *system) emitDeadLetter(dl DeadLetter) {
	if h := x.getConfig().deadLetterHandler; h != nil {
		defer func() {
			if err := recover(); err != nil {
				x.Logger().Error("dead letter handler panic",
					"owner", dl.Owner, "target", dl.Target, "reason", dl.Reason,
					"err", err, "stack", ghelper.StackTrace())
			}
		}()
		h(dl)
		return
	}
	x.Logger().Warn("dead letter",
		"owner", dl.Owner, "target", dl.Target,
		"msgName", proto.MessageName(dl.Message), "reason", dl.Reason)
}
