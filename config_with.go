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

// WithConfigGrpcDialOptions APPENDS dial options to the defaults.
//
// It used to replace the slice, which silently dropped the default
// insecure.NewCredentials() seeded in newConfig — so adding a single unrelated option
// made grpc.NewClient fail with "no transport security set", surfacing only as
// streamWriteActor logging and poisoning itself. If you need different credentials,
// pass grpc.WithTransportCredentials explicitly; the later option wins.
func WithConfigGrpcDialOptions(dialOptions ...grpc.DialOption) ConfigOptFunc {
	return func(config *config) {
		config.dialOptions = append(config.dialOptions, dialOptions...)
	}
}

// WithConfigCallDialOptions appends grpc call options.
//
// Deprecated: the name is wrong — these are CallOptions, nothing to do with dialing.
// Use WithConfigGrpcCallOptions.
func WithConfigCallDialOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return WithConfigGrpcCallOptions(callOptions...)
}

// WithConfigGrpcCallOptions appends grpc call options used for the outbound stream.
func WithConfigGrpcCallOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return func(config *config) {
		config.callOptions = append(config.callOptions, callOptions...)
	}
}

func WithConfigKind(kindName string, producer Producer, opts ...KindOptFunc) ConfigOptFunc {
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
