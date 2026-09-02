package grain

// WithOptsMailboxSize sets the mailbox's INITIAL capacity (default 8). The mailbox doubles on
// demand up to its max size, so this is only a floor: raise it to pre-reserve for a kind known to
// burst and skip the doublings. It trades memory per actor against a one-off growth cost and does
// not affect steady-state throughput. See WithOptsMailboxMaxSize for the ceiling.
func WithOptsMailboxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxInitSize = size
	}
}

// WithOptsMailboxMaxSize sets the mailbox's MAX capacity (default 4096). Once full at this size,
// further messages overflow to a dead letter instead of blocking the sender.
func WithOptsMailboxMaxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxMaxSize = size
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
