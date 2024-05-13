package actor

import (
	"context"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/actor/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log/slog"
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
	ctx, cancel := context.WithCancel(context.Background())
	x.cancelFunc = cancel
	keepAliveChan, err := etcdClient.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		return err
	}
	//lease
	x.leaseId = leaseResp.ID
	//register
	err = x.register()
	if err != nil {
		return err
	}
	//watch
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
		state := x.config.InitState(x.addr(), id)
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

func (x *ProviderEtcd) Logger() *slog.Logger {
	return x.logger
}

func (x *ProviderEtcd) ensureLocalActorExist(ref *ActorRef) {
	if ref == nil {
		x.Logger().Warn("ignore ensure, actor ref is nil")
		return
	}
	refKind := ref.GetKind()
	prod, ok := x.config.kinds[refKind]
	if !ok {
		x.Logger().Error("ignore ensure, actor ref kind not exist", "kind", refKind)
		return
	}
	//double check
	if x.system.registry.get(ref) == nil {
		x.system.SpawnNamed(prod, ref.GetName())
	}

	//todo use agent or direct registry?

	//todo 1. check get?
	//todo 2. if not found, get from kind local provider?
	//todo 2.1 if kind not found at local provider, ignore and print log
	//todo 2.2 if found kind at local provider, new kind
	//todo 2.2.1 add to this registry, use return to instead self(because may already add, for double check)
	//todo return self
}
