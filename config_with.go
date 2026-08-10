package grain

import (
	"time"

	"google.golang.org/grpc"
)

func WithConfigAskTimeout(d time.Duration) ConfigOptFunc {
	return func(config *config) {
		config.askTimeout = d
	}
}

func WithConfigStopWaitTimeSecond(t int) ConfigOptFunc {
	return func(config *config) {
		config.stopWaitTimeSecond = t
	}
}

func WithConfigGrpcDialOptions(dialOptions ...grpc.DialOption) ConfigOptFunc {
	return func(config *config) {
		config.dialOptions = dialOptions
	}
}

func WithConfigCallDialOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return func(config *config) {
		config.callOptions = callOptions
	}
}

func WithConfigKind(kindName string, producer iProducer, opts ...KindOptFunc) ConfigOptFunc {
	return func(config *config) {
		config.mustNotRunning()
		if kindName == defaultLocalKind ||
			kindName == defaultSystemKind ||
			kindName == defaultReplyKind ||
			kindName == defaultWriteStreamKind {
			panic("invalid kind name, please change")
		}
		if _, ok := config.kinds[kindName]; ok {
			panic("duplicate kind name " + kindName)
		}
		config.kinds[kindName] = tKind{producer: producer, opts: opts}
	}
}

// WithConfigDeadLetter sets a system-wide handler for undeliverable messages
// (mailbox overflow or a send to a stopped actor). When unset, dead letters are
// logged at WARN. The handler runs on the sender's goroutine, so keep it fast
// and non-blocking.
func WithConfigDeadLetter(h DeadLetterHandler) ConfigOptFunc {
	return func(config *config) {
		config.deadLetterHandler = h
	}
}
