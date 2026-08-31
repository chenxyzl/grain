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
	logger         *slog.Logger
	eventStream    ActorRef
	askId          uint64
	// addr caches rpcService.Addr() after Start(); it is immutable once the grpc
	// server is listening, so the per-Send getAddr() avoids an interface dispatch.
	addr string
	// draining is set when shutdown begins. grpc is stopped only AFTER the actors are
	// drained, so inbound envelopes keep arriving during and after the drain; without
	// this flag they activate brand-new cluster grains that nobody will ever poison,
	// so their PreStop — and any state persistence in it — never runs.
	draining atomic.Bool
	// pending holds in-flight Ask correlation channels keyed by msg snId. A reply
	// (local or routed back from a remote node) is delivered by snId, so a reply
	// no longer needs to be a registered actor.
	pending safemap.ConcurrentMap[uint64, chan proto.Message]
}

// NewSystem ...
// @param clusterName mean etcd root
// @param clusterUrls mean etcd urls
func NewSystem(clusterName string, version string, clusterUrls []string, opts ...ConfigOptFunc) ISystem {
	sys := &system{}
	sys.config = newConfig(clusterName, version, clusterUrls, opts...)
	//
	sys.logger = slog.Default()
	sys.registry = newRegistry(sys.Logger())
	sys.clusterProvider = &providerEtcd{}
	sys.forceCloseChan = make(chan bool, 1)
	sys.timerSchedule = newTimerSchedule(sys)
	sys.addrHash = newAddrHash()
	sys.pending = safemap.NewIntC[uint64, chan proto.Message]()
	//
	return sys
}
func (x *system) getAddr() string          { return x.addr }
func (x *system) getConfig() *config       { return x.config }
func (x *system) getProvider() iProvider   { return x.clusterProvider }
func (x *system) getRegistry() iRegistry   { return x.registry }
func (x *system) nextSnId() uint64         { return atomic.AddUint64(&x.askId, 1) }
func (x *system) GetScheduler() IScheduler { return x }
func (x *system) getAddrHash() *AddrHash   { return x.addrHash }

// Cluster node info and per-node ext data, forwarded to the provider. These used to
// be reached as system.GetProvider().Xxx(), i.e. exported methods on the unexported
// iProvider — callable only by accident. Now they are first-class on ISystem and the
// provider stays internal.
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

func (x *system) Logger() *slog.Logger { return x.logger }

// ErrNameExists is returned by SpawnNamed when an actor with that name is already
// registered on this node. Compare with errors.Is.
var ErrNameExists = errors.New("grain: actor name already exists")

// Spawn creates an actor under a generated unique name, so it cannot fail on a name
// collision.
func (x *system) Spawn(p Producer, opts ...KindOptFunc) ActorRef {
	// FormatUint, not Itoa(int(...)): Generate() returns uint64 and the int conversion
	// truncates on a 32-bit build, which would collide actor names.
	ref, err := x.SpawnNamed(p, strconv.FormatUint(uuid.Generate(), 10), opts...)
	if err != nil {
		// The name comes from the uuid generator, so a collision here is a broken
		// invariant in the framework, not a caller mistake.
		panic("grain: Spawn hit a name collision on a generated id, this is a bug: " + err.Error())
	}
	return ref
}

// SpawnNamed creates an actor under an explicit name, unique per node.
//
// Returns ErrNameExists if the name is taken. It used to panic instead, taking the
// whole process down for what is either a caller mistake or a benign race — respawning
// a named actor after a crash, or two goroutines racing to create it. Compare with the
// two reference designs: Akka's actorOf throws a *catchable* InvalidActorNameException,
// and Orleans sidesteps the question entirely because grains are virtual and activation
// is idempotent (which is what this framework's cluster kinds already do via
// ensureClusterKindActorExist). For an explicit named spawn in Go, protoactor-go's
// (PID, error) with ErrNameExists is the closest fit, so that is what this returns.
func (x *system) SpawnNamed(p Producer, name string, opts ...KindOptFunc) (ActorRef, error) {
	//
	opts = append(opts, withOptsDirectSelf(name, x.getAddr(), x))
	options := newOpts(p, opts...)
	//
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
	//check
	if target == nil {
		x.Logger().Error("target actor is nil")
		return
	}
	//check actor type
	if target.isDirect() {
		targetAddr := target.GetDirectAddr()
		//direct actor
		if targetAddr == x.getAddr() {
			x.sendToLocal(target, msg, sender, msgSnId)
		} else {
			x.sendToCluster(targetAddr, target, msg, sender, msgSnId)
		}
	} else {
		//for performance op
		if proc := x.registry.get(target); proc != nil {
			proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
		} else {
			//cluster actor
			cacheAddr := target.getRemoteAddrCache()
			if cacheAddr == "" {
				x.Logger().Error("actor kind not in cluster")
				return
			}
			//
			if cacheAddr == x.getAddr() {
				// This node owns the kind. On-demand activation happens inside
				// sendToLocal, after its existing poison check — see there for why.
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
	// reply target: deliver to the waiting Ask via the pending table (by snId),
	// not through the registry — a reply is a correlation-id future, not an actor.
	if target.isAsk() {
		x.deliverReply(target.askSnId(), msg)
		return
	}
	//to local
	proc := x.registry.get(target)
	if proc == nil {
		// A poison for something that is not running is a no-op. This check must come
		// BEFORE on-demand activation below: otherwise stopping a dormant grain would
		// first run its full Started() (cluster registration, state load) and then
		// PreStop() immediately, which for a grain that persists in PreStop can write
		// empty or just-loaded state over the real thing.
		if _, ok := msg.(*message.Poison); ok {
			//ignore poison msg if proc not found
			return
		}
		// Cluster kind owned by this node but not activated yet: activate on demand.
		// Done here rather than in tellWithSender so the message-type check above is
		// performed exactly once on this path instead of twice.
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
	//
	proc.send(newContext(proc.self(), sender, msg, msgSnId, x))
}

func (x *system) sendToCluster(targetAddress string, target ActorRef, msg proto.Message, sender ActorRef, msgSnId uint64) {
	//remote addr
	writeStreamActorRef := newDirectActorRef(defaultWriteStreamKind, targetAddress, x.getAddr(), x)
	//get proc, or spawn idempotently (multiple senders may race here; a
	//duplicate-id panic would crash the process, so use the get-or-create path).
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
	// local actor: use the non-blocking control signal (never blocks on a full
	// mailbox, safe under registry locks). remote actor: send poison as a normal
	// message over the network.
	if proc := x.registry.get(ref); proc != nil {
		proc.poison()
		return
	}
	x.tell(ref, poison)
}

// registerAsk allocates a correlation channel for an in-flight Ask and stores it
// under snId. The channel is buffered (cap 1) so deliverReply never blocks.
func (x *system) registerAsk(snId uint64) chan proto.Message {
	ch := make(chan proto.Message, 1)
	x.pending.Set(snId, ch)
	return ch
}

// deliverReply routes a reply to the waiting Ask by snId. It atomically removes
// the entry (Pop), so at most one of {reply, timeout, shutdown} ever writes the
// channel; a late reply whose Ask already finished finds nothing and is dropped.
func (x *system) deliverReply(snId uint64, msg proto.Message) {
	if ch, ok := x.pending.Pop(snId); ok {
		ch <- msg // cap-1 buffered, single writer wins the Pop, never blocks
	}
}

// cancelAsk removes the pending entry (Ask finished or timed out). Idempotent:
// if a reply already Popped it, this is a no-op.
func (x *system) cancelAsk(snId uint64) {
	x.pending.Remove(snId)
}

