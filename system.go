package grain

import (
	"errors"
	"log/slog"
	"strconv"
	"sync"
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
	pending                   safemap.ConcurrentMap[uint64, chan proto.Message]
	pendingRegistrationMu     sync.RWMutex
	pendingRegistrationClosed bool
	streamReferences          atomic.Pointer[streamReferenceCache]
	processorCreation         processorCreator
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
// process. Returns ErrInvalidName if the name contains an actor-id separator ('/' or '@').
func (x *system) SpawnNamed(p Producer, name string, opts ...KindOptFunc) (ActorRef, error) {
	if err := ValidateActorName(name); err != nil {
		return nil, err
	}
	opts = append(opts, withOptsDirectSelf(name, x.getAddr(), x))
	options := newOpts(p, opts...)
	proc, err := spawnNewProcessor(x, options)
	if err != nil {
		return nil, err
	}
	return proc.self(), nil
}

// GetClusterActorRef returns a reference to the cluster grain (kind, name), wherever the cluster
// currently hosts it. It does not activate anything: the first message does.
//
// PANICS on a kind or name containing an actor-id separator ('/' or '@'), because there is no error
// to return and the alternative is worse — such a ref parses into an id with every field empty, so
// every send to it is dropped and no node can ever activate it, with nothing reported. When the
// name comes from user data, screen it with ValidateActorName first.
func (x *system) GetClusterActorRef(kind string, name string) ActorRef {
	if err := validateKindName(kind); err != nil {
		panic(err.Error() + " (from GetClusterActorRef)")
	}
	if err := ValidateActorName(name); err != nil {
		panic(err.Error() + " (from GetClusterActorRef; use grain.ValidateActorName to screen " +
			"names that come from user data)")
	}
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
		if proc := x.registry.lookup(target.GetId()); proc != nil {
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
				// no mailbox was involved, so Owner is nil
				x.emitDeadLetter(DeadLetter{Target: target, Sender: sender, Message: msg,
					MsgSnId: msgSnId, Reason: DeadLetterReasonNoOwner})
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
	proc := x.registry.lookup(target.GetId())
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
			proc = x.registry.lookup(target.GetId())
		}
	}
	if proc == nil {
		if sender != nil && sender.isAsk() {
			sender.Tell(errActorNotFound)
		}
		x.Logger().Error("send, get actor failed", "actor", target, "msgName", proto.MessageName(msg))
		// A fire-and-forget Tell has no sender to notify, so without this it vanished into a log
		// line that no monitoring hook could see. This is the COMMON miss — a target that already
		// deregistered — not the narrow stop window processorMailBox.send covers.
		// Owner is nil: there was no mailbox to reject it.
		x.emitDeadLetter(DeadLetter{Target: target, Sender: sender, Message: msg,
			MsgSnId: msgSnId, Reason: DeadLetterReasonNotFound})
		return
	}
	proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
}

func (x *system) sendToCluster(targetAddress string, target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	writeStreamActorRef := x.writeStreamRef(targetAddress)
	//get, or spawn idempotently: senders race here and a duplicate-id panic kills the process
	proc := x.registry.lookup(writeStreamActorRef.GetId())
	if proc == nil {
		opts := newOpts(func() IActor {
			return newStreamWriterActor(writeStreamActorRef, targetAddress, x.getConfig().dialOptions, x.getConfig().callOptions)
		}, WithOptsKindName(writeStreamActorRef.GetKind()), WithOptsPoisonFirstOnQuit(false), withOptsDirectSelf(writeStreamActorRef.GetName(), x.getAddr(), x))
		proc = getOrSpawnProcessor(x, opts)
	}
	proc.send(newContext(target, sender, msg, msgSnId, x))
}

func (x *system) Poison(ref ActorRef) {
	// local: the non-blocking control signal (never blocks on a full mailbox, safe under
	// registry locks). remote: poison travels as an ordinary message.
	if proc := x.registry.lookup(ref.GetId()); proc != nil {
		proc.poison()
		return
	}
	x.tell(ref, msgPoison)
}

// registerAsk holds a shared admission lock through insertion. The pending map
// synchronizes concurrent writes itself; only shutdown needs exclusive admission
// to close registration and snapshot every remaining accepted waiter.
func (x *system) registerAsk(snId uint64) chan proto.Message {
	x.pendingRegistrationMu.RLock()
	defer x.pendingRegistrationMu.RUnlock()
	if x.pendingRegistrationClosed {
		return nil
	}
	ch := make(chan proto.Message, 1)
	x.pending.Set(snId, ch)
	return ch
}

// deliverReply routes a response directly to its waiter. Shutdown can already
// have filled the cap-1 channel; a late/duplicate response must never block the
// replying actor. Timed-out callers unregister before waiting to resume.
func (x *system) deliverReply(snId uint64, msg proto.Message) {
	if ch, ok := x.pending.Pop(snId); ok {
		select {
		case ch <- msg:
		default: // already carries a poison/reply nobody will read; the Ask is over
		}
	}
}

// cancelAsk removes the pending entry (Ask finished or timed out). Idempotent.
func (x *system) cancelAsk(snId uint64) {
	x.pending.Remove(snId)
}

func (x *system) getProcessorCreator() *processorCreator { return &x.processorCreation }
