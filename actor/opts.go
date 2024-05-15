package actor

import (
	"context"
	"math"
	"time"
)

const (
	defaultMailboxSize = 1024
	defaultMaxRestarts = 3
)

var (
	defaultRestartDelay = func(restartTimes int) time.Duration {
		if restartTimes < 1 {
			restartTimes = 1
		}
		return 100 * time.Millisecond * time.Duration(math.Pow(2, float64(restartTimes-1)))
	}
)

type OptFunc func(*Opts)

type Opts struct {
	Producer     Producer
	MailboxSize  int
	Kind         string
	MaxRestarts  int32
	RestartDelay func(restartTimes int) time.Duration
	Context      context.Context
	Self         *ActorRef
}

// NewOpts ...
func NewOpts(p Producer, opts ...OptFunc) Opts {
	ret := Opts{
		Producer:     p,
		MailboxSize:  defaultMailboxSize,
		Kind:         defaultLocalKind,
		MaxRestarts:  defaultMaxRestarts,
		RestartDelay: defaultRestartDelay,
		Context:      context.Background(),
	}
	for _, opt := range opts {
		opt(&ret)
	}
	return ret
}

func WithContext(ctx context.Context) OptFunc {
	return func(opts *Opts) {
		opts.Context = ctx
	}
}
func WithRestartDelay(d func(restartTimes int) time.Duration) OptFunc {
	return func(opts *Opts) {
		opts.RestartDelay = d
	}
}
func WithInboxSize(size int) OptFunc {
	return func(opts *Opts) {
		opts.MailboxSize = size
	}
}
func WithMaxRestarts(n int) OptFunc {
	return func(opts *Opts) {
		opts.MaxRestarts = int32(n)
	}
}
func WithKindName(kindName string) OptFunc {
	return func(opts *Opts) {
		opts.Kind = kindName
	}
}
func withSelf(address string, name string) OptFunc {
	return func(opts *Opts) {
		opts.Self = newActorRefWithKind(address, opts.Kind, name)
	}
}
