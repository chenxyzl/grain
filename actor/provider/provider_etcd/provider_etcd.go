package provider_etcd

import (
	"context"
	"errors"
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/actor/def"
	"github.com/chenxyzl/grain/actor/provider"
	"github.com/chenxyzl/grain/actor/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log/slog"
)

var _ provider.Provider = (*ProviderEtcd)(nil)

const ttlTime = 10

type ProviderEtcd struct {
	//
	state  def.NodeState
	system *actor.System
	config *def.Config
	//
	logger *slog.Logger

	//etcd cluster
	client     *clientv3.Client
	leaseId    clientv3.LeaseID
	cancelFunc context.CancelFunc
	listener   provider.ProviderListener

	//self rpc
	selfAddr   string
	rpcService *actor.RPCService
}

func (x *ProviderEtcd) SelfAddr() string {
	return x.selfAddr
}

func (x *ProviderEtcd) Address() string {
	return x.selfAddr
}

func (x *ProviderEtcd) Start(system *actor.System, state def.NodeState, config *def.Config, listener provider.ProviderListener) error {
	rpcService := actor.NewRpcServer(system)
	//start grpc
	if err := rpcService.Start(); err != nil {
		return err
	}
	x.state = state
	x.rpcService = rpcService
	x.selfAddr = rpcService.SelfAddr()
	x.logger = slog.With("ProviderEtcd", x.selfAddr)
	x.listener = listener
	//etcdClient
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: config.GetRemoteUrls(), DialTimeout: ttlTime})
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
				if !ok {
					x.Logger().Warn("lease expired or KeepAlive channel closed")
					if x.listener != nil {
						x.listener.ClusterErr()
					}
					return
				}
			}
		}
	}()
	return nil
}
func (x *ProviderEtcd) Stop() error {
	x.listener = nil
	err := x.client.Close()
	if err != nil {
		x.Logger().Info("cluster provider etcd stopped with err.", "err", err)
		return err
	}
	x.Logger().Info("cluster provider etcd stopped")
	return nil
}
func (x *ProviderEtcd) GetNodesByKind(kind string) []def.NodeState {
	//TODO implement me
	panic("implement me")
}

func (x *ProviderEtcd) RegisterActor(state def.ActorState) error {
	//TODO implement me
	panic("implement me")
}

func (x *ProviderEtcd) UnregisterActor(state def.ActorState) {
	//TODO implement me
	panic("implement me")
}

func (x *ProviderEtcd) register() error {
	for id := uint64(1); id <= uuid.MaxNodeMax(); id++ {
		key := x.config.GetMemberPath(id)
		//
		if !x.set(key, x.state) {
			continue
		}
		x.listener.InitGlobalUuid(id)
		//
		x.Logger().Info("register node to etcd success", "key", key, "val", x.state)
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
