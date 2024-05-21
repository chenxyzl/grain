package actor

import (
	"context"
	"time"
)

func WithOptsContext(ctx context.Context) KindOptFunc {
	return func(opts *Opts) {
		opts.Context = ctx
	}
}
func WithOptsRestartDelay(d func(restartTimes int) time.Duration) KindOptFunc {
	return func(opts *Opts) {
		opts.RestartDelay = d
	}
}
func WithOptsInboxSize(size int) KindOptFunc {
	return func(opts *Opts) {
		opts.MailboxSize = size
	}
}
func WithOptsMaxRestarts(n int) KindOptFunc {
	return func(opts *Opts) {
		opts.MaxRestarts = int32(n)
	}
}
func WithOptsRegisterToRemote(fun func(clusterProvider Provider, config *Config, ref *ActorRef)) KindOptFunc {
	return func(opts *Opts) {
		opts.RegisterToRemote = fun
	}
}
func WithOptsUnRegisterFromRemote(fun func(clusterProvider Provider, config *Config, ref *ActorRef)) KindOptFunc {
	return func(opts *Opts) {
		opts.UnRegisterFromRemote = fun
	}
}
func withOptsKindName(kindName string) KindOptFunc {
	return func(opts *Opts) {
		opts.Kind = kindName
	}
}
func withOptsSelf(address string, name string) KindOptFunc {
	return func(opts *Opts) {
		opts.Self = newActorRefWithKind(address, opts.Kind, name)
	}
}
