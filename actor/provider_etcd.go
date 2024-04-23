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

func (x *ProviderEtcd) SelfAddr() string {
	return x.selfAddr
}

func (x *ProviderEtcd) Address() string {
	return x.selfAddr
}

func (x *ProviderEtcd) Start(system *System, config *Config) error {
	rpcService := NewRpcServer(system)
	//start grpc
	if err := rpcService.Start(); err != nil {
		return err
	}

	//
	config.state = NodeState{Address: rpcService.SelfAddr(), Time: time.Now(), Version: config.version}
	x.system = system
	x.config = config
	x.rpcService = rpcService
	x.selfAddr = rpcService.SelfAddr()
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
					x.Logger().Warn("lease expired or KeepAlive channel closed")
					if x.system != nil {
						x.system.ClusterErr()
					}
					return
				}
			}
		}
	}()
	return nil
}
func (x *ProviderEtcd) Stop() error {
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
	return nil
}
func (x *ProviderEtcd) GetNodesByKind(kind string) []NodeState {
	//TODO implement me
	panic("implement me")
}

func (x *ProviderEtcd) RegisterActor(state ActorState) error {
	//TODO implement me
	panic("implement me")
}

func (x *ProviderEtcd) UnregisterActor(state ActorState) {
	//TODO implement me
	panic("implement me")
}

func (x *ProviderEtcd) register() error {
	for id := uint64(1); id <= uuid.MaxNodeMax(); id++ {
		key := x.config.GetMemberPath(id)
		x.config.state.NodeId = id
		//
		if !x.set(key, x.config.state) {
			continue
		}
		x.logger = x.logger.With("node", id)
		x.system.InitGlobalUuid(id)
		//
		x.Logger().Info("register node to etcd success", "key", key, "val", x.config.state)
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
