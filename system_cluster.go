package grain

func (x *system) clusterMemberChanged() {
	clusterNodes, clusterProviderVersion := x.clusterProvider.GetNodes()
	x.Logger().Warn("cluster node changed", "clusterProviderVersion", clusterProviderVersion)
	addr := x.getAddr()
	// rangeIt snapshots and invokes the callback outside the registry lock, so the
	// rendezvous hashing below does not run under it either.
	x.registry.rangeIt(func(key string, v iProcess) {
		self := v.self()
		//direct actor not need deal
		if self.isDirect() {
			return
		}
		//cluster actor
		newAddr := x.getAddrHash().CalcAddrByKind8Name(clusterNodes, self.GetKind(), self.GetName())
		if newAddr != "" && newAddr != addr {
			// this actor no longer belongs to this node: stop it. A stateful actor
			// should persist in PreStop and reload in Started on the new owner node.
			//
			// v.poison() rather than x.Poison(self): the process is already in hand, so
			// this skips a registry lookup and — more importantly — skips Poison's
			// "not in registry -> tell(ref, poison)" fallback, which for a cluster ref
			// routes through ensureClusterKindActorExist and would *spawn* the actor
			// just to kill it.
			v.poison()
		}
	})
}

func (x *system) ensureClusterKindActorExist(ref ActorRef) bool {
	if ref == nil {
		x.Logger().Warn("ignore ensure, actor ref is nil")
		return false
	}
	// Never activate a grain once shutdown has begun: it would be spawned after
	// stopActors already ran, so nothing poisons it and its PreStop never runs.
	if x.draining.Load() {
		x.Logger().Warn("ignore ensure, system is draining", "actor", ref)
		return false
	}
	refKind := ref.GetKind()
	kind, ok := x.config.kinds[refKind]
	//not register kind
	if !ok {
		return false
	}
	//has found
	if x.registry.get(ref) != nil {
		return true
	}
	//instant: idempotent spawn, concurrent callers must not trigger a
	//duplicate-id panic that would crash the process.
	opts := newOpts(kind.producer, append(kind.opts, withOptsClusterSelf(ref))...)
	newProcessorOrGet(x, opts)
	return true
}
