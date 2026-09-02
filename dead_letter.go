package grain

import "google.golang.org/protobuf/proto"

// Dead-letter reasons.
const (
	DeadLetterReasonOverflow = "mailbox overflow" // mailbox full and at max capacity
	DeadLetterReasonStopped  = "actor stopped"    // target actor already stopped
)

// DeadLetter describes an undeliverable message, surfaced to the DeadLetterHandler set via
// WithConfigDeadLetter (defaults to a WARN log).
type DeadLetter struct {
	Target ActorRef // intended recipient; for a cross-node send the REMOTE actor
	// Owner is the actor whose mailbox actually rejected it — on the outbound path the local
	// write_stream actor, not Target, meaning the outbound stream is the bottleneck.
	Owner   ActorRef
	Sender  ActorRef // may be nil
	Message proto.Message
	MsgSnId uint64
	Reason  string
}

// DeadLetterHandler receives undeliverable messages. Invoked on the SENDER's goroutine, so it
// must be fast and non-blocking. A panic in it is recovered and logged, not propagated.
type DeadLetterHandler func(DeadLetter)
