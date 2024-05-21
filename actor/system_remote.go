package actor

import "slices"

func (x *System) clusterMemberChanged() {
	clusterNodes := x.clusterProvider.getNodes()
	addr := x.clusterProvider.addr()
	x.registry.rangeIt(func(key string, v iProcess) {
		self := v.self()
		newAddr := x.calcAddressByKind7Id(clusterNodes, self.GetKind(), self.GetName())
		if newAddr != "" && newAddr != addr {
			x.Poison(self)
		}
	})
}

func (x *System) ensureRemoteKindActorExist(ref *ActorRef) {
	if ref == nil {
		x.Logger().Warn("ignore ensure, actor ref is nil")
		return
	}
	refKind := ref.GetKind()
	kind, ok := x.config.kinds[refKind]
	if ok && ref.GetAddress() == x.clusterProvider.addr() && x.registry.get(ref) == nil {
		x.SpawnNamed(kind.producer, ref.GetName(), append(kind.opts, WithOptsKindName(ref.GetKind()))...)
	}
}

func (x *System) calcAddressByKind7Id(clusterNodes []tNodeState, kind string, name string) string {
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
	return maxMember.Address
}

func (x *System) hash(node, key []byte) uint32 {
	x.hasherLock.Lock()
	defer x.hasherLock.Unlock()

	x.hasher.Reset()
	_, _ = x.hasher.Write(key)
	_, _ = x.hasher.Write(node)
	return x.hasher.Sum32()
}
