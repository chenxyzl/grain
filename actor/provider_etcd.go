package actor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/uuid"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log/slog"
	"strings"
	"time"
)

var _ Provider = (*ProviderEtcd)(nil)

const dialTimeoutTime = time.Second * 10
const ttlTime = 10

type ProviderEtcd struct {
	//
	system *System
	config *Config
	//
	logger *slog.Logger

	//etcd cluster
	client     *clientv3.Client
	leaseId    clientv3.LeaseID
	cancelFunc context.CancelFunc

	//self rpc
	selfAddr   string
	rpcService *rpcService

	//
	nodeMap *safemap.SafeMap[string, tNodeState]
}

func (x *ProviderEtcd) getEtcdClient() *clientv3.Client {
	return x.client
}

func (x *ProviderEtcd) getEtcdLease() clientv3.LeaseID {
	return x.leaseId
}

func (x *ProviderEtcd) Logger() *slog.Logger {
	return x.logger
}

func (x *ProviderEtcd) addr() string {
	return x.selfAddr
}

func (x *ProviderEtcd) Address() string {
	return x.selfAddr
}

func (x *ProviderEtcd) start(system *System, config *Config) error {
	gs := newRpcServer(system.sendWithSender)
	//start grpc
	if err := gs.Start(); err != nil {
		return err
	}
	//
	x.nodeMap = safemap.NewM[string, tNodeState]()
	x.system = system
	x.config = config
	x.rpcService = gs
	x.selfAddr = gs.Addr()
	x.logger = slog.With("ProviderEtcd", x.selfAddr)
	//etcdClient
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: config.GetRemoteUrls(), DialTimeout: dialTimeoutTime})
	if err != nil {
		return fmt.Errorf("cannot connect to etcd:%v|err:%v", config.GetRemoteUrls(), err)
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
	err = x.register()
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
func (x *ProviderEtcd) stop() {
	//
	x.system = nil
	//
	if x.rpcService != nil {
		err := x.rpcService.Stop()
		if err != nil {
			x.Logger().Warn("rpc service stop err", "err", err)
		}
	}
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
func (x *ProviderEtcd) register() error {
	for id := uint64(1); id <= uuid.MaxNodeMax(); id++ {
		key := x.config.GetMemberPath(id)
		s, _ := json.Marshal(x.config.init(x.addr(), id))
		state := string(s)
		//
		if !x.setTxn(key, state) {
			continue
		}
		x.logger = x.logger.With("node", id)
		x.system.running(id)
		//
		x.Logger().Info("register node to etcd success", "key", key, "val", state)
		return nil
	}
	return errors.New("register node to etcd error")
}

// setTxn set Key=val if key not exist
func (x *ProviderEtcd) setTxn(key string, val string) bool {
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
func (x *ProviderEtcd) removeTxn(key string, val string) bool {
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

// SetNodeExtData set node ext data, keep life with node
func (x *ProviderEtcd) SetNodeExtData(subKey string, val string) error {
	key := x.config.GetMemberExtDataPath(x.config.state.NodeId) + "/" + subKey
	_, err := x.client.Put(context.Background(), key, val, clientv3.WithLease(x.leaseId))
	return err
}

// RemoveNodeExtData remove node ext date
func (x *ProviderEtcd) RemoveNodeExtData(subKey string) error {
	key := x.config.GetMemberExtDataPath(x.config.state.NodeId) + "/" + subKey
	_, err := x.client.Delete(context.Background(), key)
	return err
}

func (x *ProviderEtcd) keepAlive(keepAliveChan <-chan *clientv3.LeaseKeepAliveResponse) {
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
func (x *ProviderEtcd) watch() error {
	//first
	rsp, err := x.client.Get(context.Background(), x.config.GetMemberPrefix(), clientv3.WithPrefix())
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
	wch := x.client.Watch(context.Background(), x.config.GetMemberPrefix(), clientv3.WithPrefix(), clientv3.WithPrevKV())
	go func() {
		for v := range wch {
			for _, kv := range v.Events {
				_ = x.parseWatch(kv.Type, string(kv.Kv.Key), kv.Kv.Value)
			}
			if x.system != nil {
				x.system.clusterMemberChanged()
			}
		}
	}()
	return nil
}

func (x *ProviderEtcd) parseWatch(op mvccpb.Event_EventType, key string, value []byte) (err error) {
	arr := strings.Split(key, "/")
	if len(arr) > 0 {
		key = arr[len(arr)-1]
	}
	if op == mvccpb.DELETE {
		x.nodeMap.Delete(key)
		return nil
	}
	a := tNodeState{}
	if err = json.Unmarshal(value, &a); err != nil {
		x.nodeMap.Delete(key)
		x.Logger().Error("watcher key changed, bug parse err, remove node", "node", key, "v", string(value), "err", err)
	} else {
		x.nodeMap.Set(key, a)
		x.Logger().Warn("watcher key changed, success", "key", key, "v", a)
	}
	return err
}

func (x *ProviderEtcd) getNodes() []tNodeState {
	var nodes []tNodeState
	x.nodeMap.Range(func(s string, state tNodeState) {
		nodes = append(nodes, state)
	})
	return nodes
}
