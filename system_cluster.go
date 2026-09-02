package grain

func (x *system) clusterMemberChanged() {
	clusterNodes, clusterProviderVersion := x.clusterProvider.GetNodes()
	x.Logger().Warn("cluster node changed", "clusterProviderVersion", clusterProviderVersion)
	addr := x.getAddr()
	// rangeIt invokes the callback outside the registry lock, so the hashing below is unlocked
	x.registry.rangeIt(func(key string, v iProcess) {
		self := v.self()
		if self.isDirect() {
			return
		}
		newAddr := x.getAddrHash().CalcAddrByKind8Name(clusterNodes, self.GetKind(), self.GetName())
		if newAddr != "" && newAddr != addr {
			// no longer owned by this node: stop it (a stateful actor persists in PreStop and
			// reloads in Started on the new owner). v.poison(), not x.Poison(self): that would
			// hit Poison's "not in registry -> tell(poison)" fallback, which for a cluster ref
			// spawns the actor just to kill it.
			v.poison()
		}
	})
}

func (x *system) ensureClusterKindActorExist(ref ActorRef) bool {
	if ref == nil {
		x.Logger().Warn("ignore ensure, actor ref is nil")
		return false
	}
	// never activate once shutdown began: nothing would poison it, so PreStop never runs
	if x.draining.Load() {
		x.Logger().Warn("ignore ensure, system is draining", "actor", ref)
		return false
	}
	refKind := ref.GetKind()
	kind, ok := x.config.kinds[refKind]
	if !ok {
		return false
	}
	if x.registry.get(ref) != nil {
		return true
	}
	//idempotent spawn: concurrent callers must not hit a duplicate-id panic
	opts := newOpts(kind.producer, append(kind.opts, withOptsClusterSelf(ref))...)
	newProcessorOrGet(x, opts)
	return true
}
