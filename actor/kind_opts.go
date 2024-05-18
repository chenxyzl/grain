package actor

import (
	"context"
	"math"
	"slices"
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
	defaultRegisterToRemote = func(clusterProvider Provider, config *Config, ref *ActorRef) {
		//register to remote
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			times := 0
			registerSuccess := false
			for {
				times++
				if times >= 2 {
					time.Sleep(time.Millisecond * 100 * (1 << (times - 2)))
				}
				if times > defaultRegisterTimes {
					break
				}
				if !clusterProvider.setTxn(config.GetRemoteActorKind(ref), ref.GetAddress()) {
					continue
				}
				//
				registerSuccess = true
				break
			}
			if !registerSuccess {
				panic("failed register remote actor to clusterProvider")
			}
		}
	}
	defaultUnregisterFromRemote = func(clusterProvider Provider, config *Config, ref *ActorRef) {
		//unRegister from remote
		if slices.Contains(config.state.Kinds, ref.GetKind()) {
			if clusterProvider.removeTxn(config.GetRemoteActorKind(ref), ref.GetAddress()) {
			}
		}
	}
)

type KindOptFunc func(*Opts)

type Opts struct {
	Producer     Producer
	MailboxSize  int
	Kind         string
	MaxRestarts  int32
	RestartDelay func(restartTimes int) time.Duration
	Context      context.Context
	Self         *ActorRef

	RegisterToRemote     func(clusterProvider Provider, config *Config, ref *ActorRef)
	UnRegisterFromRemote func(clusterProvider Provider, config *Config, ref *ActorRef)
}

// NewOpts ...
func NewOpts(p Producer, opts ...KindOptFunc) Opts {
	ret := Opts{
		Producer:             p,
		MailboxSize:          defaultMailboxSize,
		Kind:                 defaultLocalKind,
		MaxRestarts:          defaultMaxRestarts,
		RestartDelay:         defaultRestartDelay,
		Context:              context.Background(),
		RegisterToRemote:     defaultRegisterToRemote,
		UnRegisterFromRemote: defaultUnregisterFromRemote,
	}
	for _, opt := range opts {
		opt(&ret)
	}
	return ret
}

func WithContext(ctx context.Context) KindOptFunc {
	return func(opts *Opts) {
		opts.Context = ctx
	}
}
func WithRestartDelay(d func(restartTimes int) time.Duration) KindOptFunc {
	return func(opts *Opts) {
		opts.RestartDelay = d
	}
}
func WithInboxSize(size int) KindOptFunc {
	return func(opts *Opts) {
		opts.MailboxSize = size
	}
}
func WithMaxRestarts(n int) KindOptFunc {
	return func(opts *Opts) {
		opts.MaxRestarts = int32(n)
	}
}
func WithRegisterToRemote(fun func(clusterProvider Provider, config *Config, ref *ActorRef)) KindOptFunc {
	return func(opts *Opts) {
		opts.RegisterToRemote = fun
	}
}
func WithUnRegisterFromRemote(fun func(clusterProvider Provider, config *Config, ref *ActorRef)) KindOptFunc {
	return func(opts *Opts) {
		opts.UnRegisterFromRemote = fun
	}
}
func withKindName(kindName string) KindOptFunc {
	return func(opts *Opts) {
		opts.Kind = kindName
	}
}
func withSelf(address string, name string) KindOptFunc {
	return func(opts *Opts) {
		opts.Self = newActorRefWithKind(address, opts.Kind, name)
	}
}
