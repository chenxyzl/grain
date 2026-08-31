package grain

// WithOptsMailboxSize sets the mailbox's INITIAL capacity (default 8). The mailbox grows
// on demand (doubling) up to its max size, so this is only a floor — raise it to
// pre-reserve for a kind known to arrive in bursts and skip the doublings. It does not
// affect steady-state throughput (measured identical from 1 to 512 slots); it trades
// memory per actor against a one-off ~1us growth cost for actors that queue deeply.
// Use WithOptsMailboxMaxSize to set the ceiling.
func WithOptsMailboxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxInitSize = size
	}
}

// WithOptsMailboxMaxSize sets the mailbox's MAX capacity (default 4096). Once the
// mailbox is full at this size, further messages overflow to a dead letter
// instead of blocking the sender.
func WithOptsMailboxMaxSize(size int) KindOptFunc {
	return func(opts *tOpts) {
		opts.mailboxMaxSize = size
	}
}

// NOTE: WithOptsRegisterToCluster / WithOptsUnRegisterFromCluster were removed.
// They had zero call sites AND were impossible to call from outside the package: the
// callback took iProvider and *config, both unexported, so no external closure could
// be written. Reinstating them as a real extension point requires exporting those
// types first.

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
