package grain

import "slices"

func (x *system) clusterMemberChanged() {
	clusterNodes, clusterProviderVersion := x.clusterProvider.getNodes()
	x.Logger().Warn("cluster node changed", "clusterProviderVersion", clusterProviderVersion)
	addr := x.getAddr()
	x.registry.rangeIt(func(key string, v iProcess) {
		self := v.self()
		//direct actor not need deal
		if self.isDirect() {
			return
		}
		//cluster actor
		newAddr := x.calcAddressByKind8Id(clusterNodes, self.GetKind(), self.GetName())
		if newAddr != "" && newAddr != addr {
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
	//instant
	x.SpawnClusterName(kind.producer, append(kind.opts, withOptsClusterSelf(ref))...)
	return true
}

func (x *system) calcAddressByKind8Id(clusterNodes []tNodeState, kind string, name string) string {
	var nodes []tNodeState
	for _, state := range clusterNodes {
		if slices.Contains(state.Kinds, kind) {
			nodes = append(nodes, state)
		}
	}
	l := len(nodes)
	if l == 0 {
		return ""
	}
	if l == 1 {
		return nodes[0].Address
	}
	keyBytes := []byte(name)
	var maxScore uint32
	var maxMember *tNodeState
	var score uint32
	for _, node := range nodes {
		score = x.hash([]byte(node.Address), keyBytes)
		if score > maxScore {
			maxScore = score
			maxMember = &node
		}
	}
	//maxMember will not nil
	if maxMember == nil {
		return ""
	}
	return maxMember.Address
}

func (x *system) hash(node, key []byte) uint32 {
	x.hasherLock.Lock()
	defer x.hasherLock.Unlock()

	x.hasher.Reset()
	_, _ = x.hasher.Write(key)
	_, _ = x.hasher.Write(node)
	return x.hasher.Sum32()
}
