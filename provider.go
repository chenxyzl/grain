package grain

import (
	"log/slog"
	"reflect"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type iProvider interface {
	//etcd
	getEtcdClient() *clientv3.Client
	getEtcdLease() clientv3.LeaseID

	//life
	start(systemLife iSystemLife, clusterMemberChangedListener func(), addr string, config *config, logger *slog.Logger) error
	stop()

	//nodes
	getNodes() ([]tNodeState, int64)

	//set remove key val
	setTxn(key string, val string) bool
	removeTxn(key string, val string) bool

	//getNodeExtData get node ext data
	getNodeExtData(subKey string) (string, error)
	//setNodeExtData set node ext data, keep life with node
	setNodeExtData(subKey string, val string) error
	//removeNodeExtData remove node ext date
	removeNodeExtData(subKey string) error
	//watchNodeExtData watch node ext data, if val == "", mean`s delete
	watchNodeExtData(subKey string, f func(key, val string)) error
}

func newProvider[T iProvider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
