package actor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/al/safemap"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"hash"
	"hash/fnv"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

var _ Provider = (*ProviderEtcd)(nil)
var _ ProviderListener = (*System)(nil)

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
	rpcService *RPCService

	//
	nodeMap *safemap.SafeMap[string, NodeState]
	//
	hasher     hash.Hash32
	hasherLock sync.Mutex
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
	rpcService := NewRpcServer(system)
	//start grpc
	if err := rpcService.Start(); err != nil {
		return err
	}
	//
	x.hasher = fnv.New32a()
	x.nodeMap = safemap.NewM[string, NodeState]()
	x.system = system
	x.config = config
	x.rpcService = rpcService
	x.selfAddr = rpcService.Addr()
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
	//register
	err = x.register()
	if err != nil {
		return err
	}
	//watcher
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
		s, _ := json.Marshal(x.config.InitState(x.addr(), id))
		state := string(s)
		//
		if !x.set(key, state) {
			continue
		}
		x.logger = x.logger.With("node", id)
		x.system.InitGlobalUuid(id)
		//
		x.Logger().Info("register node to etcd success", "key", key, "val", state)
		return nil
	}
	return errors.New("register node to etcd error")
}

func (x *ProviderEtcd) set(key string, val any) bool {
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
						x.system.ClusterErr()
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
	a := NodeState{}
	if err = json.Unmarshal(value, &a); err != nil {
		x.nodeMap.Delete(key)
		x.Logger().Error("watcher key changed, bug parse err, remove node", "node", key, "v", string(value), "err", err)
	} else {
		x.nodeMap.Set(key, a)
		x.Logger().Warn("watcher key changed, success", "key", key, "v", a)
	}
	return err
}

func (x *ProviderEtcd) getNodes() []NodeState {
	var nodes []NodeState
	x.nodeMap.Range(func(s string, state NodeState) {
		nodes = append(nodes, state)
	})
	return nodes
}

func (x *ProviderEtcd) ensureRemoteKindActorExist(ref *ActorRef) {
	if ref == nil {
		x.Logger().Warn("ignore ensure, actor ref is nil")
		return
	}
	refKind := ref.GetKind()
	prod, ok := x.config.kinds[refKind]
	if ok && ref.GetAddress() == x.Address() && x.system.registry.get(ref) == nil {
		x.system.SpawnNamed(prod, ref.GetName(), WithKindName(ref.GetKind()))
	}
}

func (x *ProviderEtcd) getAddressByKind7Id(kind string, name string) string {
	var nodes []NodeState
	x.nodeMap.Range(func(_ string, state NodeState) {
		if slices.Contains(state.Kinds, kind) {
			nodes = append(nodes, state)
		}
	})
	l := len(nodes)
	if l == 0 {
		return ""
	}
	if l == 1 {
		return nodes[0].Address
	}
	keyBytes := []byte(name)
	var maxScore uint32
	var maxMember *NodeState
	var score uint32
	for _, node := range nodes {
		score = x.hash([]byte(node.Address), keyBytes)
		if score > maxScore {
			maxScore = score
			maxMember = &node
		}
	}

	if maxMember == nil {
		return ""
	}
	return maxMember.Address
}

func (r *ProviderEtcd) hash(node, key []byte) uint32 {
	r.hasherLock.Lock()
	defer r.hasherLock.Unlock()

	r.hasher.Reset()
	_, _ = r.hasher.Write(key)
	_, _ = r.hasher.Write(node)
	return r.hasher.Sum32()
}
