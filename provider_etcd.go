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

	// stopping is set by stop() before it starts tearing etcd down. The background
	// goroutines (keepAlive, the watches) check it to tell "we are shutting down" from
	// "etcd failed", which they otherwise cannot distinguish — a closed keepalive
	// channel looks identical either way.
	//
	// This replaces the old `x.system = nil` sentinel, which was an unsynchronized
	// write to an interface field read by the keepAlive goroutine: a data race, and
	// one with a real nil-call window between its check and its use.
	stopping atomic.Bool

	//
	nodeChangeLocker sync.RWMutex
	// localProviderVersion is bumped on every node-set change. Read lock-free via
	// GetNodesVersion on the cluster-send hot path; written under nodeChangeLocker
	// (which also guards nodeMap) so version and map stay consistent.
	localProviderVersion atomic.Int64
	nodeMap              map[string]tNodeState
}

// registerEventStream puts path=val bound to this node's lease (keeps the
// subscription entry alive with the node).
func (x *providerEtcd) registerEventStream(path string, val string) error {
	_, err := x.client.Put(context.Background(), path, val, clientv3.WithLease(x.leaseId))
	return err
}

// unregisterEventStream deletes the subscription entry at path.
func (x *providerEtcd) unregisterEventStream(path string) error {
	_, err := x.client.Delete(context.Background(), path)
	return err
}

// watchEventStream loads the current entries under prefix (as watchPut) then
// watches for changes, mapping etcd's mvccpb type to the neutral watchOp so the
// caller (eventStream) stays free of etcd types.
func (x *providerEtcd) watchEventStream(prefix string, f func(op watchOp, key string, val []byte)) error {
	rsp, err := x.client.Get(context.Background(), prefix, clientv3.WithPrefix())
	if err != nil {
		return errors.Join(err, errors.New("first load eventStream err"))
	}
	for _, kv := range rsp.Kvs {
		f(watchPut, string(kv.Key), kv.Value)
	}
	// anchor the watch to the snapshot revision (+1) so no change made between the
	// Get above and the Watch below is lost; see etcd's Range+Watch atomic pattern.
	wch := x.client.Watch(context.Background(), prefix, clientv3.WithPrefix(), clientv3.WithRev(rsp.Header.Revision+1))
	go func() {
		for v := range wch {
			// A terminated watch (compacted revision, auth revocation, server-side
			// cancel) arrives as one response whose Err() is set, and the channel closes
			// right after. Returning here does not cut a live watch short — it is already
			// dead; the plain `for range` used to fall out of the loop anyway, just
			// without recording why, freezing the subscription view silently.
			//
			// Our own teardown closes the client, which surfaces the same way, so check
			// stopping first: otherwise every clean shutdown logs a bogus ERROR.
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
	//
	x.logger = logger
	x.localProviderVersion.Store(1) // 0 is direct[local] actor
	x.nodeMap = make(map[string]tNodeState)
	x.system = systemLife
	x.clusterMemberChangedListener = clusterMemberChangedListener
	x.config = config
	//etcdClient
	etcdClient, err := clientv3.New(clientv3.Config{Endpoints: config.getClusterUrls(), DialTimeout: config.etcdDialTimeout})
	if err != nil {
		return fmt.Errorf("cannot connect to etcd:%v|err:%v", config.getClusterUrls(), err)
	}
	x.client = etcdClient
	// Every failure below used to return while leaving the client open, the lease
	// alive, the keepalive goroutine running — and, once register() had run, this
	// node's member key published in etcd. Peers would then route real traffic for up
	// to the lease TTL to a node that never finished starting. Revoking the lease also
	// deletes the member key, since it is written WithLease.
	started := false
	defer func() {
		if !started {
			x.releaseEtcd()
		}
	}()
	//lease and keep alive
	leaseResp, err := etcdClient.Grant(context.Background(), x.config.etcdLeaseTTLSecond)
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
	// Load the member set FIRST, then register, then start the watch. The old order was
	// register -> watch, and watch did its own Get, so between publishing our member key
	// and that Get landing, nodeMap was empty while grpc was already accepting (Start
	// listens before it calls us). Any cluster envelope arriving in that window resolved to
	// no owner and was dropped. Doing the Get up front also means there is only one.
	rev, err := x.loadMembers()
	if err != nil {
		return err
	}
	//register self
	err = x.register(addr)
	if err != nil {
		return err
	}
	//watcher nodes
	err = x.watchMembers(rev)
	if err != nil {
		return err
	}
	//keep
	x.keepAlive(keepAliveChan)
	started = true
	return nil
}

// releaseEtcd tears down whatever start() managed to acquire: the keepalive context,
// the lease (which also removes every key bound to it, this node's member entry
// included) and the client. Idempotent, and shared by stop() and start()'s failure
// path.
func (x *providerEtcd) releaseEtcd() {
	// cancelFunc stops the keepalive goroutine's context. It was previously assigned
	// and then never called anywhere, so the keepalive only unwound as a side effect
	// of closing the client.
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
	// Tell the background goroutines this is a deliberate teardown BEFORE tearing
	// anything down, so they do not mistake it for an etcd failure and try to stop the
	// system again.
	x.stopping.Store(true)
	x.releaseEtcd()
	x.Logger().Info("cluster provider etcd stopped")
}

// registerRounds bounds the claim retries in register(); a round is only consumed by
// genuinely losing a race.
const registerRounds = 16

// register claims a node id with one create-only Txn on a free id, retrying (after
// refreshing the member set) if it loses.
//
// It used to walk id = 1, 2, 3 ... with a Txn per candidate, so the Nth node to join paid N
// round-trips — 200 nodes cost sum(1..200) ~ 20,100 transactions. loadMembers() has already
// told us what is free, so the common case is one Txn.
//
// Two things worth knowing: that snapshot is only a HINT (correctness rests entirely on
// setTxn's create-only compare, so a stale view costs a round, never a duplicate id), and
// the candidate is RANDOM rather than lowest-free — nodes booting together read
// near-identical snapshots, so lowest-free aims them all at the same id and the retry chain
// is O(N) rounds all over again.
func (x *providerEtcd) register(addr string) error {
	for round := 0; round < registerRounds; round++ {
		free := x.freeNodeIds()
		if len(free) == 0 {
			// Distinguished from a lost race on purpose: "the cluster is full" is an
			// operational limit (uuid's node field is 10 bits), not a transient failure, and
			// the old code reported both as the same generic error.
			return fmt.Errorf("register node to etcd error: all %d node ids are in use",
				uuid.MaxNodeMax())
		}
		id := free[rand.IntN(len(free))]
		key := x.config.getMemberPath(id)
		s, _ := json.Marshal(x.config.init(addr, id))
		state := string(s)
		if !x.setTxn(key, state) {
			// Lost the race, or etcd errored (setTxn cannot tell the caller which, but it
			// logs the etcd case). Either way our view is stale now, so refresh it.
			if _, err := x.loadMembers(); err != nil {
				return err
			}
			continue
		}
		// Add ourselves to nodeMap now instead of waiting for our own watch event:
		// CalcAddrByKind8Name must see this node among its own candidates, or until that
		// round-trip lands we route our own grains to peers.
		x.parseWatch(mvccpb.PUT, key, s)
		x.logger = x.logger.With("node", id)
		x.system.init(id)
		//
		x.Logger().Info("register node to etcd success", "key", key, "val", state,
			"rounds", round+1)
		return nil
	}
	return fmt.Errorf("register node to etcd error: lost the id race %d times running",
		registerRounds)
}

// freeNodeIds lists the ids in 1..MaxNodeMax that nodeMap does not show as taken. nodeMap is
// already keyed by the id (parseWatch keeps the last path segment), so nothing needs parsing.
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

// setTxn set Key=val if key not exist
//
// Returns false both when the key is already taken (a lost race, expected) and when
// etcd itself failed. The two are indistinguishable to the caller, which is why the
// etcd error is logged HERE: without it, a brief etcd outage during register() looks
// exactly like "every node id from 1..1023 is in use", and the caller reports the
// generic "register node to etcd error" with the real cause nowhere to be found.
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

// removeTxn remove key if getValue(key) == val
//
// As with setTxn, false covers both "value did not match" and "etcd failed", so the
// etcd error is logged here to keep the real cause visible.
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
			// `stopping` is what distinguishes them (it replaces the old, racy
			// `x.system == nil` sentinel).
			if !x.stopping.Load() && x.system != nil {
				x.Logger().Warn("lease expired or KeepAlive channel closed")
				x.system.ForceStop(fmt.Errorf("cluster provider error. will stop system"))
			}
			return
		}
	}()
}

// loadMembers does the one Get of the member prefix that both the free-id search and the
// watch are built on, filling nodeMap from it. Returns the snapshot revision so the watch
// can anchor at rev+1 and miss nothing in between.
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

// watchMembers starts the member watch at rev+1, where rev is the loadMembers() snapshot —
// so a change made between the two is delivered rather than lost.
func (x *providerEtcd) watchMembers(rev int64) error {
	wch := x.client.Watch(context.Background(), x.config.getMemberPrefix(), clientv3.WithPrefix(), clientv3.WithRev(rev+1))
	go func() {
		for v := range wch {
			// A terminated watch (compacted revision, auth revocation, server-side
			// cancel) arrives as one response whose Err() is set, and the channel closes
			// right after — so returning here does not cut a live watch short. The plain
			// `for range` used to fall straight through, leaving this node serving on a
			// frozen member set forever: it keeps routing cluster actors to nodes that
			// have left and never learns of new ones, silently and with no log.
			// Misrouting is worse than being down, so stop the system instead — the same
			// escalation the lease-expiry path already uses.
			//
			// Our own teardown closes the client, which surfaces identically, so check
			// stopping first.
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
			//listener
			x.clusterMemberChangedListener()
		}
		if !x.stopping.Load() {
			x.Logger().Error("member watch channel closed unexpectedly, stopping system to avoid routing on a stale member set")
			x.stopSystemOnWatchLoss()
		}
	}()
	return nil
}

// stopSystemOnWatchLoss escalates an unrecoverable loss of the member watch. Guarded
// by `stopping` so a teardown-induced close does not try to stop the system again.
//
// TODO: re-establishing the watch (fresh Get + Watch, rebuilding nodeMap from
// scratch so departed nodes do not linger) would be preferable to stopping. Until
// then, failing loudly beats silently routing on a stale view.
func (x *providerEtcd) stopSystemOnWatchLoss() {
	if x.stopping.Load() {
		return
	}
	if x.system != nil {
		x.system.ForceStop(errors.New("cluster member watch lost. will stop system"))
	}
}

// parseWatch applies one member-key change to nodeMap.
//
// It never returns an error: a malformed value is logged and the node dropped, which
// is all a caller could do anyway. It used to return the json error *after* handling
// it, and watch()'s initial load propagated that up through start() into a panic in
// system.Start() — so a single garbage or legacy-format member value written by any
// other node bricked every node that tried to join, while the live watch path
// discarded the identical error with `_ =`.
func (x *providerEtcd) parseWatch(op mvccpb.Event_EventType, key string, value []byte) {
	x.nodeChangeLocker.Lock()
	defer x.nodeChangeLocker.Unlock()
	x.localProviderVersion.Add(1)
	//
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
	// lock-free: version is atomic, so the cluster-send hot path avoids the RLock.
	return x.localProviderVersion.Load()
}
func (x *providerEtcd) GetNodes() ([]tNodeState, int64) {
	x.nodeChangeLocker.RLock()
	defer x.nodeChangeLocker.RUnlock()
	//
	version := x.localProviderVersion.Load()
	var nodes []tNodeState
	for _, state := range x.nodeMap {
		nodes = append(nodes, state)
	}
	return nodes, version
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
	// anchor to the snapshot revision (+1) so ext-data changes between the Get and
	// the Watch are not lost.
	wch := x.client.Watch(context.Background(), key, clientv3.WithPrefix(), clientv3.WithRev(rsp.Header.Revision+1))
	go func() {
		for v := range wch {
			// Terminal for this watch; the channel closes right after. Guarded by
			// stopping so our own teardown does not log a bogus ERROR.
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
