package grain

import (
	"log/slog"
)

// watchOp is the framework-neutral change type delivered by watchEventStream, so eventStream
// does not depend on etcd's mvccpb.
type watchOp int8

const (
	watchPut watchOp = iota
	watchDelete
)

type iProvider interface {
	//life
	start(systemLife iSystemLife, clusterMemberChangedListener func(), addr string, config *config, logger *slog.Logger) error
	stop()

	//nodes
	GetNodeId() uint64
	GetNodes() ([]tNodeState, int64)
	//GetNodesVersion returns the version alone, no node slice; cheap enough to poll on the
	//hot send path.
	GetNodesVersion() int64

	//set remove key val
	setTxn(key string, val string) bool
	removeTxn(key string, val string) bool

	//event stream (subscription registry) — keeps clientv3/mvccpb out of eventStream
	//registerEventStream puts path=val bound to this node's lease
	registerEventStream(path string, val string) error
	//unregisterEventStream deletes path
	unregisterEventStream(path string) error
	//watchEventStream does an initial full load then watches prefix, calling f per change.
	watchEventStream(prefix string, f func(op watchOp, key string, val []byte)) error

	//GetNodeExtData get node ext data
	GetNodeExtData(subKey string) (string, error)
	//SetNodeExtData set node ext data, keep life with node
	SetNodeExtData(subKey string, val string) error
	//RemoveNodeExtData remove node ext date
	RemoveNodeExtData(subKey string) error
	//WatchNodeExtData watch node ext data, if val == "", mean`s delete
	WatchNodeExtData(subKey string, f func(key, val string)) error
}
