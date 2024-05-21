package actor

import (
	"google.golang.org/grpc"
	"time"
)

func WithConfigRequestTimeout(d time.Duration) ConfigOptFunc {
	return func(config *Config) {
		config.requestTimeout = d
	}
}

func WithConfigStopWaitTimeSecond(t int) ConfigOptFunc {
	return func(config *Config) {
		config.stopWaitTimeSecond = t
	}
}

func WithConfigGrpcDialOptions(dialOptions ...grpc.DialOption) ConfigOptFunc {
	return func(config *Config) {
		config.dialOptions = dialOptions
	}
}

func WithConfigCallDialOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return func(config *Config) {
		config.callOptions = callOptions
	}
}

func WithConfigKind(kindName string, producer Producer, opts ...KindOptFunc) ConfigOptFunc {
	return func(config *Config) {
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
		config.kinds[kindName] = Kind{producer: producer, opts: opts}
	}
}
