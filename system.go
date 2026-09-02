package grain

import (
	"errors"
	"log/slog"
	"strconv"
	"sync/atomic"

	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/message"
	"github.com/chenxyzl/grain/uuid"
	"google.golang.org/protobuf/proto"
)

type system struct {
	config *config
	//
	registry        *registry
	rpcService      iRpcServer
	clusterProvider iProvider
	timerSchedule   *timerSchedule
	addrHash        *AddrHash

	//
	forceCloseChan chan bool
	// atomic: Start()/init() build it while grpc is ALREADY serving, so an inbound envelope's
	// RecvEnvelope -> Logger() races those writes. nil means "not built yet"; see Logger().
	logger      atomic.Pointer[slog.Logger]
	eventStream ActorRef

	// askId is atomically incremented per send while addr and logger are READ per send; one
	// shared 64-byte line is false sharing (24.1 vs 14.2 ns/op, 16 cores). One system per
	// process, so the padding is free; a field added above askId undoes it (see system_test).
	_     [64]byte
	askId uint64
	_     [56]byte
	// addr caches rpcService.Addr(); immutable once grpc is listening, so getAddr() is a read.
	addr string
	// draining is set when shutdown begins: grpc stops only AFTER actors drain, so inbound
	// envelopes must not activate new grains nobody poisons (their PreStop never runs).
	draining atomic.Bool
	// pending holds in-flight Ask reply channels keyed by msg snId, so a reply needs no actor.
	pending safemap.ConcurrentMap[uint64, chan proto.Message]
}

// NewSystem ...
// @param clusterName mean etcd root
// @param clusterUrls mean etcd urls
func NewSystem(clusterName string, version string, clusterUrls []string, opts ...ConfigOptFunc) ISystem {
	sys := &system{}
	sys.config = newConfig(clusterName, version, clusterUrls, opts...)
	// left nil when unconfigured, so Logger() resolves the process default per call
	if l := sys.config.logger; l != nil {
		sys.logger.Store(l)
	}
	sys.registry = newRegistry()
	sys.clusterProvider = &providerEtcd{}
	sys.forceCloseChan = make(chan bool, 1)
	sys.timerSchedule = newTimerSchedule(sys)
	sys.addrHash = newAddrHash()
	sys.pending = safemap.NewIntC[uint64, chan proto.Message]()
	return sys
}
func (x *system) getAddr() string          { return x.addr }
func (x *system) getConfig() *config       { return x.config }
func (x *system) getProvider() iProvider   { return x.clusterProvider }
func (x *system) getRegistry() iRegistry   { return x.registry }
func (x *system) nextSnId() uint64         { return atomic.AddUint64(&x.askId, 1) }
func (x *system) GetScheduler() IScheduler { return x }
func (x *system) getAddrHash() *AddrHash   { return x.addrHash }

// Cluster node info and per-node ext data, forwarded to the provider, which stays internal.
func (x *system) GetNodeId() uint64 { return x.clusterProvider.GetNodeId() }
func (x *system) GetNodeExtData(subKey string) (string, error) {
	return x.clusterProvider.GetNodeExtData(subKey)
}
func (x *system) SetNodeExtData(subKey string, val string) error {
	return x.clusterProvider.SetNodeExtData(subKey, val)
}
func (x *system) RemoveNodeExtData(subKey string) error {
	return x.clusterProvider.RemoveNodeExtData(subKey)
}
func (x *system) WatchNodeExtData(subKey string, f func(key, val string)) error {
	return x.clusterProvider.WatchNodeExtData(subKey, f)
}

// Logger returns the system logger, tagged system=<addr> by Start() and node=<id> by init();
// every actor logger derives from it. Without WithConfigLogger it resolves slog.Default() PER
// CALL until Start() builds it, so an InitLog issued before Start still applies — but nothing
// rebuilds it afterwards, so a later slog.SetDefault does NOT reach the framework.
func (x *system) Logger() *slog.Logger {
	if l := x.logger.Load(); l != nil {
		return l
	}
	return slog.Default()
}

// ErrNameExists is returned by SpawnNamed when the name is taken on this node (use errors.Is).
var ErrNameExists = errors.New("grain: actor name already exists")

// Spawn creates an actor under a generated unique name, so it cannot fail on a collision.
func (x *system) Spawn(p Producer, opts ...KindOptFunc) ActorRef {
	// FormatUint, not Itoa(int(...)): the int conversion truncates on a 32-bit build.
	ref, err := x.SpawnNamed(p, strconv.FormatUint(uuid.Generate(), 10), opts...)
	if err != nil {
		// a generated-uuid collision is a broken framework invariant, not a caller mistake
		panic("grain: Spawn hit a name collision on a generated id, this is a bug: " + err.Error())
	}
	return ref
}

// SpawnNamed creates an actor under an explicit name, unique per node. Returns ErrNameExists
// if it is taken: a respawn after a crash, or a race, is recoverable and must not kill the
// process.
func (x *system) SpawnNamed(p Producer, name string, opts ...KindOptFunc) (ActorRef, error) {
	opts = append(opts, withOptsDirectSelf(name, x.getAddr(), x))
	options := newOpts(p, opts...)
	proc, err := newProcessor(x, options)
	if err != nil {
		return nil, err
	}
	return proc.self(), nil
}

func (x *system) GetClusterActorRef(kind string, name string) ActorRef {
	return newClusterActorRef(kind, name, x)
}

func (x *system) getSender() iSender {
	return x
}

func (x *system) tellWithSender(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	if target.isDirect() {
		targetAddr := target.GetDirectAddr()
		if targetAddr == x.getAddr() {
			x.sendToLocal(target, msg, sender, msgSnId)
		} else {
			x.sendToCluster(targetAddr, target, msg, sender, msgSnId)
		}
	} else {
		//fast path: already registered locally
		if proc := x.registry.get(target); proc != nil {
			proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
		} else {
			cacheAddr := target.getRemoteAddrCache()
			if cacheAddr == "" {
				// Fail a waiting Ask NOW rather than wait out askTimeout: "no node hosts this
				// kind" is as decidable here as a missing actor or a dead mailbox.
				if sender != nil && sender.isAsk() {
					sender.Tell(errKindNotInCluster)
				}
				x.Logger().Error("actor kind not in cluster",
					"actor", target, "msgName", proto.MessageName(msg))
				return
			}
			if cacheAddr == x.getAddr() {
				// this node owns the kind; sendToLocal activates on demand, after its poison check
				x.sendToLocal(target, msg, sender, msgSnId)
			} else {
				x.sendToCluster(cacheAddr, target, msg, sender, msgSnId)
			}
		}
	}
}

func (x *system) tell(target ActorRef, msg proto.Message) {
	x.tellWithSender(target, msg, nil, x.nextSnId())
}

func (x *system) sendToLocal(target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	// reply target: delivered by snId via the pending table — a reply is a future, not an actor
	if target.isAsk() {
		x.deliverReply(target.askSnId(), msg)
		return
	}
	proc := x.registry.get(target)
	if proc == nil {
		// Poison for a non-running actor is a no-op, and must be checked BEFORE the on-demand
		// activation below: else stopping a dormant grain runs its full Started() then
		// PreStop(), which for a grain that persists in PreStop overwrites its real state.
		if _, ok := msg.(*message.Poison); ok {
			return
		}
		// cluster kind owned by this node but not yet activated: activate on demand. Here
		// rather than in tellWithSender so the message-type check above runs once, not twice.
		if target.isCluster() && x.ensureClusterKindActorExist(target) {
			proc = x.registry.get(target)
		}
	}
	if proc == nil {
		if sender != nil && sender.isAsk() {
			sender.Tell(errActorNotFound)
		}
		x.Logger().Error("send, get actor failed", "actor", target, "msgName", proto.MessageName(msg))
		return
	}
	proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
}

func (x *system) sendToCluster(targetAddress string, target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	writeStreamActorRef := newDirectActorRef(defaultWriteStreamKind, targetAddress, x.getAddr(), x)
	//get, or spawn idempotently: senders race here and a duplicate-id panic kills the process
	proc := x.registry.get(writeStreamActorRef)
	if proc == nil {
		opts := newOpts(func() IActor {
			return newStreamWriterActor(writeStreamActorRef, targetAddress, x.getConfig().dialOptions, x.getConfig().callOptions)
		}, WithOptsKindName(writeStreamActorRef.GetKind()), WithOptsPoisonFirstOnQuit(false), withOptsDirectSelf(writeStreamActorRef.GetName(), x.getAddr(), x))
		proc = newProcessorOrGet(x, opts)
	}
	proc.send(newContext(target, sender, msg, msgSnId, x))
}

func (x *system) Poison(ref ActorRef) {
	// local: the non-blocking control signal (never blocks on a full mailbox, safe under
	// registry locks). remote: poison travels as an ordinary message.
	if proc := x.registry.get(ref); proc != nil {
		proc.poison()
		return
	}
	x.tell(ref, msgPoison)
}

// registerAsk stores a cap-1 correlation channel for an in-flight Ask, keyed by snId.
func (x *system) registerAsk(snId uint64) chan proto.Message {
	ch := make(chan proto.Message, 1)
	x.pending.Set(snId, ch)
	return ch
}

// deliverReply routes a reply to the waiting Ask by snId. Pop is atomic, so at most one of
// {reply, timeout, shutdown} ever writes the channel; a late reply finds nothing and is dropped.
func (x *system) deliverReply(snId uint64, msg proto.Message) {
	if ch, ok := x.pending.Pop(snId); ok {
		ch <- msg // cap-1, single writer wins the Pop, never blocks
	}
}

// cancelAsk removes the pending entry (Ask finished or timed out). Idempotent.
func (x *system) cancelAsk(snId uint64) {
	x.pending.Remove(snId)
}
