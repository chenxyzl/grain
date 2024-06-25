package actor

import (
	clientv3 "go.etcd.io/etcd/client/v3"
	"reflect"
)

type Provider interface {
	//
	addr() string

	//etcd
	getEtcdClient() *clientv3.Client
	getEtcdLease() clientv3.LeaseID

	//life
	start(system *System, config *Config) error
	stop()

	//nodes
	getNodes() []tNodeState

	//set remove key val
	setTxn(key string, val string) bool
	removeTxn(key string, val string) bool

	//GetNodeExtData get node ext data
	GetNodeExtData(subKey string) (string, error)
	//SetNodeExtData set node ext data, keep life with node
	SetNodeExtData(subKey string, val string) error
	//RemoveNodeExtData remove node ext date
	RemoveNodeExtData(subKey string) error
	//WatchNodeExtData watch node ext data, if val == "", mean`s delete
	WatchNodeExtData(subKey string, f func(key, val string)) error
}

func newProvider[T Provider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
