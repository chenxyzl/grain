package grain

// WithOptsInboxSize sets the mailbox's INITIAL capacity (default 128). The
// mailbox grows on demand up to its max size; use WithOptsInboxMaxSize to set
// the ceiling.
func WithOptsInboxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxInitSize = size
	}
}

// WithOptsInboxMaxSize sets the mailbox's MAX capacity (default 4096). Once the
// mailbox is full at this size, further messages overflow to a dead letter
// instead of blocking the sender.
func WithOptsInboxMaxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxMaxSize = size
	}
}
func WithOptsRegisterToCluster(fun func(clusterProvider iProvider, config *config, ref ActorRef) error) KindOptFunc {
	return func(opts *tOpts) {
		opts.registerToCluster = fun
	}
}
func WithOptsUnRegisterFromCluster(fun func(clusterProvider iProvider, config *config, ref ActorRef)) KindOptFunc {
	return func(opts *tOpts) {
		opts.unRegisterFromCluster = fun
	}
}
func WithOptsPoisonFirstOnQuit(poisonFirstOnQuit bool) KindOptFunc {
	return func(opts *tOpts) {
		opts.poisonFirstOnQuit = poisonFirstOnQuit
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
