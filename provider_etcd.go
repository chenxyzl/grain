package grain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chenxyzl/grain/uuid"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var _ iProvider = (*providerEtcd)(nil)

type providerEtcd struct {
	config *config

	clusterMemberChangedListener func()
	system                       iSystemLife

	logger *slog.Logger

	client     *clientv3.Client
	leaseId    clientv3.LeaseID
	cancelFunc context.CancelFunc

	// stopping lets the background goroutines (keepAlive, the watches) tell a deliberate teardown
	// from an etcd failure: a closed keepalive channel or a killed watch looks identical either way.
	stopping atomic.Bool

	nodeChangeLocker sync.RWMutex
	// localProviderVersion is bumped on every node-set change. Read lock-free via
	// GetNodesVersion on the cluster-send hot path; written under nodeChangeLocker so it stays
	// consistent with nodeMap.
	localProviderVersion atomic.Int64
	nodeMap              map[string]tNodeState
}

// registerEventStream puts path=val bound to this node's lease, so the entry dies with the node.
func (x *providerEtcd) registerEventStream(path string, val string) error {
	_, err := x.client.Put(context.Background(), path, val, clientv3.WithLease(x.leaseId))
	return err
}

// unregisterEventStream deletes the subscription entry at path.
func (x *providerEtcd) unregisterEventStream(path string) error {
	_, err := x.client.Delete(context.Background(), path)
	return err
}

// watchEventStream replays the current entries under prefix as watchPut, then watches from that
// snapshot's rev+1, mapping mvccpb types to the neutral watchOp so eventStream stays etcd-free.
func (x *providerEtcd) watchEventStream(prefix string, f func(op watchOp, key string, val []byte)) error {
	rsp, err := x.client.Get(context.Background(), prefix, clientv3.WithPrefix())
	if err != nil {
		return errors.Join(err, errors.New("first load eventStream err"))
	}
	for _, kv := range rsp.Kvs {
		f(watchPut, string(kv.Key), kv.Value)
	}
	wch := x.client.Watch(context.Background(), prefix, clientv3.WithPrefix(), clientv3.WithRev(rsp.Header.Revision+1))
	go func() {
		for v := range wch {
			// terminal for this watch, and `stopping`-guarded: our own teardown looks identical
			if err := v.Err(); err != nil {
				if !x.stopping.Load() {
					x.Logger().Error("eventStream watch terminated by etcd; subscriptions are now frozen",
						"prefix", prefix, "canceled", v.Canceled, "err", err)
				}
				return
			}
			for _, kv := range v.Events {
				op := watchPut
				if kv.Type == mvccpb.DELETE {
					op = watchDelete
				}
				f(op, string(kv.Kv.Key), kv.Kv.Value)
			}
		}
		if !x.stopping.Load() {
			x.Logger().Error("eventStream watch channel closed unexpectedly; subscriptions are now frozen",
				"prefix", prefix)
		}
	}()
	return nil
}

func (x *providerEtcd) Logger() *slog.Logger {
	return x.logger
}

func (x *providerEtcd) start(systemLife iSystemLife, clusterMemberChangedListener func(), addr string, config *config, logger *slog.Logger) error {
	x.logger = logger
	x.localProviderVersion.Store(1) // 0 is direct[local] actor
	x.nodeMap = make(map[string]tNodeState)
	x.system = systemLife
	x.clusterMemberChangedListener = clusterMemberChangedListener
	x.config = config
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: config.getClusterUrls(), DialTimeout: config.etcdDialTimeout})
	if err != nil {
		return fmt.Errorf("cannot connect to etcd:%v|err:%v", config.getClusterUrls(), err)
	}
	x.client = etcdClient
	// No failure below may leave the client, lease and keepalive running with this node's
	// member key published: peers would route real traffic for up to the lease TTL to a node
	// that never finished starting. Revoking the lease deletes the member key with it.
	started := false
	defer func() {
		if !started {
			x.releaseEtcd()
		}
	}()
	leaseResp, err := etcdClient.Grant(context.Background(), x.config.etcdLeaseTTLSecond)
	if err != nil {
		return err
	}
	x.leaseId = leaseResp.ID
	ctx, cancel := context.WithCancel(context.Background())
	x.cancelFunc = cancel
	keepAliveChan, err := etcdClient.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		return err
	}
	// Load the member set FIRST, then register, then watch. grpc is already accepting when
	// Start calls us, so any window where our member key is published while nodeMap is still
	// empty resolves cluster envelopes to no owner and drops them.
	rev, err := x.loadMembers()
	if err != nil {
		return err
	}
	err = x.register(addr)
	if err != nil {
		return err
	}
	err = x.watchMembers(rev)
	if err != nil {
		return err
	}
	x.keepAlive(keepAliveChan)
	started = true
	return nil
}

// releaseEtcd undoes whatever start() acquired: the keepalive context, the lease (which also
// removes every key bound to it, this node's member entry included) and the client.
// Idempotent, and shared by stop() and start()'s failure path.
func (x *providerEtcd) releaseEtcd() {
	if x.cancelFunc != nil {
		x.cancelFunc()
		x.cancelFunc = nil
	}
	if x.client != nil && x.leaseId != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), x.config.etcdDialTimeout)
		defer cancel()
		if _, err := x.client.Revoke(ctx, x.leaseId); err != nil {
			x.Logger().Info("cluster provider etcd revoke lease err", "err", err)
		}
		x.leaseId = 0
	}
	if x.client != nil {
		if err := x.client.Close(); err != nil {
			x.Logger().Info("cluster provider etcd close client err", "err", err)
		}
		x.client = nil
	}
}

func (x *providerEtcd) stop() {
	// flag the deliberate teardown BEFORE tearing anything down, so the background goroutines do
	// not mistake it for an etcd failure and stop the system again
	x.stopping.Store(true)
	x.releaseEtcd()
	x.Logger().Info("cluster provider etcd stopped")
}

// registerRounds bounds register()'s claim retries; a round is only consumed by losing a race.
const registerRounds = 16

// register claims a node id with one create-only Txn on a free id, retrying (after refreshing
// the member set) if it loses. The free-id snapshot is only a HINT — correctness rests on
// setTxn's create-only compare, so a stale view costs a round, never a duplicate id. The
// candidate is RANDOM, not lowest-free: nodes booting together read near-identical snapshots,
// so lowest-free would aim them all at the same id and make the retry chain O(N) again.
func (x *providerEtcd) register(addr string) error {
	for round := 0; round < registerRounds; round++ {
		free := x.freeNodeIds()
		if len(free) == 0 {
			// not a lost race but an operational limit: uuid's node field is 10 bits
			return fmt.Errorf("register node to etcd error: all %d node ids are in use",
				uuid.MaxNodeMax())
		}
		id := free[rand.IntN(len(free))]
		key := x.config.getMemberPath(id)
		s, _ := json.Marshal(x.config.init(addr, id))
		state := string(s)
		if !x.setTxn(key, state) {
			// Lost the race, or etcd errored (setTxn logs that); either way our view is stale.
			if _, err := x.loadMembers(); err != nil {
				return err
			}
			continue
		}
		// Add ourselves now instead of waiting for our own watch event: CalcAddrByKind8Name
		// must see this node among its candidates, or we route our own grains to peers.
		x.parseWatch(mvccpb.PUT, key, s)
		x.logger = x.logger.With("node", id)
		x.system.init(id)
		x.Logger().Info("register node to etcd success", "key", key, "val", state,
			"rounds", round+1)
		return nil
	}
	return fmt.Errorf("register node to etcd error: lost the id race %d times running",
		registerRounds)
}

// freeNodeIds lists the ids in 1..MaxNodeMax that nodeMap does not show as taken; nodeMap is
// already keyed by id (parseWatch keeps the last path segment), so nothing needs parsing.
func (x *providerEtcd) freeNodeIds() []uint64 {
	x.nodeChangeLocker.RLock()
	defer x.nodeChangeLocker.RUnlock()

	free := make([]uint64, 0, uuid.MaxNodeMax())
	for id := uint64(1); id <= uuid.MaxNodeMax(); id++ {
		if _, taken := x.nodeMap[strconv.FormatUint(id, 10)]; !taken {
			free = append(free, id)
		}
	}
	return free
}

// setTxn puts key=val only if key does not exist. False covers both a lost race (expected) and
// an etcd failure, which the caller cannot tell apart — hence logging the etcd error HERE, or a
// brief outage during register() looks exactly like "every node id is in use".
func (x *providerEtcd) setTxn(key string, val string) bool {
	tx := x.client.Txn(context.Background())
	tx.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
		Then(clientv3.OpPut(key, fmt.Sprintf("%v", val), clientv3.WithLease(x.leaseId))).
		Else()
	txnRes, err := tx.Commit()
	if err != nil {
		x.Logger().Error("setTxn failed with an etcd error (NOT a lost race)", "key", key, "err", err)
		return false
	}
	if !txnRes.Succeeded { //抢锁失败: key already exists
		return false
	}
	return true
}

// removeTxn deletes key if its current value equals val. As with setTxn, false covers both a
// value mismatch and an etcd error, so the etcd error is logged here.
func (x *providerEtcd) removeTxn(key string, val string) bool {
	tx := x.client.Txn(context.Background())
	tx.If(clientv3.Compare(clientv3.Value(key), "=", val)).
		Then(clientv3.OpDelete(key)).
		Else()
	txnRes, err := tx.Commit()
	if err != nil {
		x.Logger().Error("removeTxn failed with an etcd error (NOT a value mismatch)", "key", key, "err", err)
		return false
	}
	if !txnRes.Succeeded { //抢锁失败: value did not match
		return false
	}
	return true
}

func (x *providerEtcd) keepAlive(keepAliveChan <-chan *clientv3.LeaseKeepAliveResponse) {
	go func() {
		for {
			_, ok := <-keepAliveChan
			if ok {
				//x.Logger().Info("etcd alive")
				continue
			}
			// The channel closes both on a genuine lease loss and on our own teardown;
			// `stopping` is what distinguishes them.
			if !x.stopping.Load() && x.system != nil {
				x.Logger().Warn("lease expired or KeepAlive channel closed")
				x.system.ForceStop(fmt.Errorf("cluster provider error. will stop system"))
			}
			return
		}
	}()
}

// loadMembers does the single Get of the member prefix that both the free-id search and the
// watch are built on, filling nodeMap from it. Returns the snapshot revision so the watch can
// anchor at rev+1 and miss nothing in between.
func (x *providerEtcd) loadMembers() (int64, error) {
	rsp, err := x.client.Get(context.Background(), x.config.getMemberPrefix(), clientv3.WithPrefix())
	if err != nil {
		return 0, errors.Join(err, errors.New("first load node state err"))
	}
	for _, kv := range rsp.Kvs {
		x.parseWatch(mvccpb.PUT, string(kv.Key), kv.Value)
	}
	return rsp.Header.Revision, nil
}

// watchMembers starts the member watch at rev+1, rev being loadMembers()'s snapshot revision.
func (x *providerEtcd) watchMembers(rev int64) error {
	wch := x.client.Watch(context.Background(), x.config.getMemberPrefix(), clientv3.WithPrefix(), clientv3.WithRev(rev+1))
	go func() {
		for v := range wch {
			// A terminated watch is TERMINAL: one response with Err() set, then the channel closes.
			// Falling through would freeze this node's member set and misroute silently, which is
			// worse than being down, so escalate. `stopping`-guarded: our teardown looks identical.
			if err := v.Err(); err != nil {
				if !x.stopping.Load() {
					x.Logger().Error("member watch terminated by etcd, stopping system to avoid routing on a stale member set",
						"canceled", v.Canceled, "err", err)
					x.stopSystemOnWatchLoss()
				}
				return
			}
			for _, kv := range v.Events {
				x.parseWatch(kv.Type, string(kv.Kv.Key), kv.Kv.Value)
			}
			x.clusterMemberChangedListener()
		}
		if !x.stopping.Load() {
			x.Logger().Error("member watch channel closed unexpectedly, stopping system to avoid routing on a stale member set")
			x.stopSystemOnWatchLoss()
		}
	}()
	return nil
}

// stopSystemOnWatchLoss escalates an unrecoverable loss of the member watch. Guarded by
// `stopping` so a teardown-induced close does not try to stop the system again.
// TODO: re-establish it instead (fresh Get + Watch, rebuilding nodeMap from scratch so departed
// nodes do not linger).
func (x *providerEtcd) stopSystemOnWatchLoss() {
	if x.stopping.Load() {
		return
	}
	if x.system != nil {
		x.system.ForceStop(errors.New("cluster member watch lost. will stop system"))
	}
}

// parseWatch applies one member-key change to nodeMap. It never returns an error: a malformed
// value is logged and that node dropped, else one garbage value written by any peer bricks every
// joining node.
func (x *providerEtcd) parseWatch(op mvccpb.Event_EventType, key string, value []byte) {
	x.nodeChangeLocker.Lock()
	defer x.nodeChangeLocker.Unlock()
	x.localProviderVersion.Add(1)
	arr := strings.Split(key, "/")
	if len(arr) > 0 {
		key = arr[len(arr)-1]
	}
	if op == mvccpb.DELETE {
		delete(x.nodeMap, key)
		return
	}
	a := tNodeState{}
	if err := json.Unmarshal(value, &a); err != nil {
		delete(x.nodeMap, key)
		x.Logger().Error("watcher key changed, but parse err, remove node", "node", key, "v", string(value), "err", err)
		return
	}
	x.nodeMap[key] = a
	x.Logger().Info("watcher key changed, success", "key", key, "v", a)
}

func (x *providerEtcd) GetNodeId() uint64 { return x.config.state.NodeId }
func (x *providerEtcd) GetNodesVersion() int64 {
	// lock-free: version is atomic, so the cluster-send hot path avoids the RLock
	return x.localProviderVersion.Load()
}
func (x *providerEtcd) GetNodes() ([]tNodeState, int64) {
	x.nodeChangeLocker.RLock()
	defer x.nodeChangeLocker.RUnlock()
	version := x.localProviderVersion.Load()
	var nodes []tNodeState
	for _, state := range x.nodeMap {
		nodes = append(nodes, state)
	}
	return nodes, version
}

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

// SetNodeExtData writes node ext data bound to this node's lease, so it dies with the node.
func (x *providerEtcd) SetNodeExtData(subKey string, val string) error {
	key := x.config.getMemberExtDataPath(subKey, x.config.state.NodeId)
	_, err := x.client.Put(context.Background(), key, val, clientv3.WithLease(x.leaseId))
	return err
}

func (x *providerEtcd) RemoveNodeExtData(subKey string) error {
	key := x.config.getMemberExtDataPath(subKey, x.config.state.NodeId)
	_, err := x.client.Delete(context.Background(), key)
	return err
}

func (x *providerEtcd) WatchNodeExtData(subKey string, f func(key, val string)) error {
	key := x.config.getMemberExtDataPath(subKey)
	rsp, err := x.client.Get(context.Background(), key, clientv3.WithPrefix())
	if err != nil {
		return errors.Join(err, errors.New("first load node ext data err"))
	}
	for _, kv := range rsp.Kvs {
		f(string(kv.Key), string(kv.Value))
	}
	wch := x.client.Watch(context.Background(), key, clientv3.WithPrefix(), clientv3.WithRev(rsp.Header.Revision+1))
	go func() {
		for v := range wch {
			// terminal for this watch, and `stopping`-guarded: see watchMembers
			if err := v.Err(); err != nil {
				if !x.stopping.Load() {
					x.Logger().Error("node ext-data watch terminated by etcd; updates are now frozen",
						"key", key, "canceled", v.Canceled, "err", err)
				}
				return
			}
			for _, kv := range v.Events {
				f(string(kv.Kv.Key), string(kv.Kv.Value))
			}
		}
		if !x.stopping.Load() {
			x.Logger().Error("node ext-data watch channel closed unexpectedly; updates are now frozen", "key", key)
		}
	}()
	return nil
}
