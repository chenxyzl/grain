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
	Target  ActorRef
	Sender  ActorRef // may be nil
	Message proto.Message
	MsgSnId uint64
	Reason  string
}

// DeadLetterHandler receives undeliverable messages. It is invoked on the
// sender's goroutine, so it must be non-blocking and fast (offload heavy work).
// A panic in the handler is recovered and logged, not propagated to the sender.
type DeadLetterHandler func(DeadLetter)
