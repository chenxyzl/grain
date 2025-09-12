package grain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/chenxyzl/grain/uuid"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var _ iProvider = (*providerEtcd)(nil)

const dialTimeoutTime = time.Second * 10
const ttlTime = 10

type providerEtcd struct {
	//
	config *config
	//
	clusterMemberChangedListener func()
	system                       iSystemLife
	//
	logger *slog.Logger

	//etcd cluster
	client     *clientv3.Client
	leaseId    clientv3.LeaseID
	cancelFunc context.CancelFunc

	//
	nodeChangeLocker     sync.RWMutex
	localProviderVersion int64
	nodeMap              map[string]tNodeState
	//nodeMap              *safemap.RWMap[string, tNodeState]
}

func (x *providerEtcd) getEtcdClient() *clientv3.Client {
	return x.client
}

func (x *providerEtcd) getEtcdLease() clientv3.LeaseID {
	return x.leaseId
}

func (x *providerEtcd) Logger() *slog.Logger {
	return x.logger
}

func (x *providerEtcd) start(systemLife iSystemLife, clusterMemberChangedListener func(), addr string, config *config, logger *slog.Logger) error {
	//
	x.logger = logger
	x.localProviderVersion = 1 // 0 is direct[local] actor
	x.nodeMap = make(map[string]tNodeState)
	//x.nodeMap = safemap.NewRWMap[string, tNodeState]()
	x.system = systemLife
	x.clusterMemberChangedListener = clusterMemberChangedListener
	x.config = config
	//etcdClient
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: config.getClusterUrls(), DialTimeout: dialTimeoutTime})
	if err != nil {
		return fmt.Errorf("cannot connect to etcd:%v|err:%v", config.getClusterUrls(), err)
	}
	x.client = etcdClient
	//lease and keep alive
	leaseResp, err := etcdClient.Grant(context.Background(), ttlTime)
	if err != nil {
		return err
	}
	//lease
	x.leaseId = leaseResp.ID
	//keep
	ctx, cancel := context.WithCancel(context.Background())
	x.cancelFunc = cancel
	keepAliveChan, err := etcdClient.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		return err
	}
	//register self
	err = x.register(addr)
	if err != nil {
		return err
	}
	//watcher nodes
	err = x.watch()
	if err != nil {
		return err
	}
	//keep
	x.keepAlive(keepAliveChan)
	return nil
}
func (x *providerEtcd) stop() {
	//
	x.system = nil // because keepAliveChan can not distinguish whether it exits normally
	//
	if x.leaseId != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), dialTimeoutTime)
		defer cancel()
		if _, err := x.client.Revoke(ctx, x.leaseId); err != nil {
			x.Logger().Info("cluster provider etcd stopped with err.", "err", err)
		}
	}
	if x.client != nil {
		if err := x.client.Close(); err != nil {
			x.Logger().Info("cluster provider etcd stopped with err.", "err", err)
		}
	}
	x.Logger().Info("cluster provider etcd stopped")
}
func (x *providerEtcd) register(addr string) error {
	for id := uint64(1); id <= uuid.MaxNodeMax(); id++ {
		key := x.config.getMemberPath(id)
		s, _ := json.Marshal(x.config.init(addr, id))
		state := string(s)
		//
		if !x.setTxn(key, state) {
			continue
		}
		x.logger = x.logger.With("node", id)
		x.system.init(id)
		//
		x.Logger().Info("register node to etcd success", "key", key, "val", state)
		return nil
	}
	return errors.New("register node to etcd error")
}

// setTxn set Key=val if key not exist
func (x *providerEtcd) setTxn(key string, val string) bool {
	tx := x.client.Txn(context.Background())
	tx.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
		Then(clientv3.OpPut(key, fmt.Sprintf("%v", val), clientv3.WithLease(x.leaseId))).
		Else()
	txnRes, err := tx.Commit()
	if err != nil || !txnRes.Succeeded { //抢锁失败
		return false
	}
	return true
}

// removeTxn remove key if getValue(key) == val
func (x *providerEtcd) removeTxn(key string, val string) bool {
	tx := x.client.Txn(context.Background())
	tx.If(clientv3.Compare(clientv3.Value(key), "=", val)).
		Then(clientv3.OpDelete(key)).
		Else()
	txnRes, err := tx.Commit()
	if err != nil || !txnRes.Succeeded { //抢锁失败
		return false
	}
	return true
}

func (x *providerEtcd) keepAlive(keepAliveChan <-chan *clientv3.LeaseKeepAliveResponse) {
	go func() {
		for {
			select {
			case _, ok := <-keepAliveChan:
				if ok {
					//x.Logger().Info("etcd alive")
				} else {
					if x.system != nil {
						x.Logger().Warn("lease expired or KeepAlive channel closed")
						x.system.ForceStop(fmt.Errorf("cluster provider error. will stop system"))
					}
					return
				}
			}
		}
	}()
}
func (x *providerEtcd) watch() error {
	//first
	rsp, err := x.client.Get(context.Background(), x.config.getMemberPrefix(), clientv3.WithPrefix())
	if err != nil {
		return errors.Join(err, errors.New("first load node state err"))
	}
	for _, kv := range rsp.Kvs {
		err = x.parseWatch(mvccpb.PUT, string(kv.Key), kv.Value)
		if err != nil {
			return err
		}
	}
	//real watch
	wch := x.client.Watch(context.Background(), x.config.getMemberPrefix(), clientv3.WithPrefix(), clientv3.WithPrevKV())
	go func() {
		for v := range wch {
			for _, kv := range v.Events {
				_ = x.parseWatch(kv.Type, string(kv.Kv.Key), kv.Kv.Value)
			}
			//listener
			x.clusterMemberChangedListener()
		}
	}()
	return nil
}

func (x *providerEtcd) parseWatch(op mvccpb.Event_EventType, key string, value []byte) (err error) {
	x.nodeChangeLocker.Lock()
	defer x.nodeChangeLocker.Unlock()
	x.localProviderVersion++
	//
	arr := strings.Split(key, "/")
	if len(arr) > 0 {
		key = arr[len(arr)-1]
	}
	if op == mvccpb.DELETE {
		delete(x.nodeMap, key)
		//x.nodeMap.Delete(key)
		return nil
	}
	a := tNodeState{}
	if err = json.Unmarshal(value, &a); err != nil {
		delete(x.nodeMap, key)
		//x.nodeMap.Delete(key)
		x.Logger().Error("watcher key changed, bug parse err, remove node", "node", key, "v", string(value), "err", err)
	} else {
		x.nodeMap[key] = a
		//x.nodeMap.Set(key, a)
		x.Logger().Warn("watcher key changed, success", "key", key, "v", a)
	}
	return err
}

func (x *providerEtcd) getNodes() ([]tNodeState, int64) {
	x.nodeChangeLocker.RLock()
	defer x.nodeChangeLocker.RUnlock()
	//
	var localProviderVersion = x.localProviderVersion
	var nodes []tNodeState
	for _, state := range x.nodeMap {
		nodes = append(nodes, state)
	}
	//x.nodeMap.Range(func(key string, state tNodeState) bool {
	//	nodes = append(nodes, state)
	//	return true
	//})
	return nodes, localProviderVersion
}

// GetNodeExtData set node ext data, keep life with node
func (x *providerEtcd) GetNodeExtData(subKey string) (string, error) {
	key := x.config.getMemberExtDataPath(subKey, x.config.state.NodeId)
	rsp, err := x.client.Get(context.Background(), key)
	if err != nil {
		return "", err
	}
	for _, kv := range rsp.Kvs {
		return string(kv.Value), nil
	}
	return "", err
}

// SetNodeExtData set node ext data, keep life with node
func (x *providerEtcd) SetNodeExtData(subKey string, val string) error {
	key := x.config.getMemberExtDataPath(subKey, x.config.state.NodeId)
	_, err := x.client.Put(context.Background(), key, val, clientv3.WithLease(x.leaseId))
	return err
}

// RemoveNodeExtData remove node ext date
func (x *providerEtcd) RemoveNodeExtData(subKey string) error {
	key := x.config.getMemberExtDataPath(subKey, x.config.state.NodeId)
	_, err := x.client.Delete(context.Background(), key)
	return err
}

// WatchNodeExtData remove node ext date
func (x *providerEtcd) WatchNodeExtData(subKey string, f func(key, val string)) error {
	key := x.config.getMemberExtDataPath(subKey)
	//first
	rsp, err := x.client.Get(context.Background(), key, clientv3.WithPrefix())
	if err != nil {
		return errors.Join(err, errors.New("first load node ext data err"))
	}
	for _, kv := range rsp.Kvs {
		f(string(kv.Key), string(kv.Value))
	}
	//real watch
	wch := x.client.Watch(context.Background(), key, clientv3.WithPrefix(), clientv3.WithPrevKV())
	go func() {
		for v := range wch {
			for _, kv := range v.Events {
				f(string(kv.Kv.Key), string(kv.Kv.Value))
			}
		}
	}()
	return nil
}
