package grain

func (x *system) clusterMemberChanged() {
	clusterNodes, clusterProviderVersion := x.clusterProvider.GetNodes()
	x.Logger().Warn("cluster node changed", "clusterProviderVersion", clusterProviderVersion)
	addr := x.getAddr()
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
			x.Poison(self)
		}
	})
}

func (x *system) ensureClusterKindActorExist(ref ActorRef) bool {
	if ref == nil {
		x.Logger().Warn("ignore ensure, actor ref is nil")
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
