package grain

import (
	"context"
	"time"
)

func WithOptsContext(ctx context.Context) KindOptFunc {
	return func(opts *tOpts) {
		opts.context = ctx
	}
}
func WithOptsRestartDelay(d func(restartTimes int) time.Duration) KindOptFunc {
	return func(opts *tOpts) {
		opts.restartDelay = d
	}
}
func WithOptsInboxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxSize = size
	}
}
func WithOptsMaxRestarts(n int) KindOptFunc {
	return func(opts *tOpts) {
		opts.maxRestarts = int32(n)
	}
}
func WithOptsRegisterToCluster(fun func(clusterProvider iProvider, config *config, ref ActorRef)) KindOptFunc {
	return func(opts *tOpts) {
		opts.registerToCluster = fun
	}
}
func WithOptsUnRegisterFromCluster(fun func(clusterProvider iProvider, config *config, ref ActorRef)) KindOptFunc {
	return func(opts *tOpts) {
		opts.unRegisterFromCluster = fun
	}
}
func WithOptsKindName(kindName string) KindOptFunc {
	return func(opts *tOpts) {
		opts.kind = kindName
	}
}
func withOptsDirectSelf(name string, address string, system ISystem) KindOptFunc {
	return func(opts *tOpts) {
		opts._self = newDirectActorRef(opts.kind, name, address, system)
	}
}
func withOptsClusterSelf(actorRef ActorRef) KindOptFunc {
	return func(opts *tOpts) {
		opts._self = actorRef
		opts.kind = opts._self.GetKind()
	}
}
