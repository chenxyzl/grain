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

	//SetNodeExtData set node ext data, keep life with node
	SetNodeExtData(subKey string, val string) error
	// RemoveNodeExtData remove node ext date
	RemoveNodeExtData(subKey string) error
}

func newProvider[T Provider]() T {
	var a T
	var t = reflect.TypeOf(a)
	return reflect.New(t.Elem()).Interface().(T)
}
