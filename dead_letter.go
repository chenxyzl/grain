package grain

import "google.golang.org/protobuf/proto"

// Dead-letter reasons.
const (
	DeadLetterReasonOverflow = "mailbox overflow" // mailbox full and at max capacity
	DeadLetterReasonStopped  = "actor stopped"    // target actor already stopped
)

// DeadLetter describes a message that could not be delivered to its target
// mailbox. It is surfaced to the DeadLetterHandler configured via
// WithConfigDeadLetter (defaults to a WARN log).
type DeadLetter struct {
	// Target is the message's intended recipient. For a cross-node send this is the
	// REMOTE actor, which is not the mailbox that overflowed — see Owner.
	Target ActorRef
	// Owner is the actor whose mailbox actually rejected the message. It differs from
	// Target on the outbound path: sendToCluster builds the context with the remote
	// target but pushes into the local write_stream actor's mailbox, so an overflow
	// there means the outbound stream is the bottleneck. Without this you could not
	// tell which of the two was saturated.
	Owner   ActorRef
	Sender  ActorRef // may be nil
	Message proto.Message
	MsgSnId uint64
	Reason  string
}

// DeadLetterHandler receives undeliverable messages. It is invoked on the
// sender's goroutine, so it must be non-blocking and fast (offload heavy work).
// A panic in the handler is recovered and logged, not propagated to the sender.
type DeadLetterHandler func(DeadLetter)
