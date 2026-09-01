package grain

import (
	"log/slog"
	"strconv"
	"time"

	"google.golang.org/grpc"
)

func WithConfigAskTimeout(d time.Duration) ConfigOptFunc {
	return func(config *config) {
		config.askTimeout = d
	}
}

func WithConfigStopWaitTimeSecond(t int) ConfigOptFunc {
	return func(config *config) {
		config.stopWaitTimeSecond = t
	}
}

// WithConfigEtcdDialTimeout bounds the initial etcd connect and the lease Revoke on
// shutdown. Default 10s.
//
// It panics on a non-positive value: clientv3 treats a zero DialTimeout as "no
// timeout", so a mistake here turns a wrong etcd endpoint from a startup error into an
// indefinite hang, which is far harder to diagnose than a panic at config time.
func WithConfigEtcdDialTimeout(d time.Duration) ConfigOptFunc {
	return func(config *config) {
		if d <= 0 {
			panic("grain: WithConfigEtcdDialTimeout needs a positive duration, got " + d.String())
		}
		config.etcdDialTimeout = d
	}
}

// WithConfigEtcdLeaseTTLSecond sets the TTL of the etcd lease this node's member key
// and event-stream subscriptions hang off. Default 10s.
//
// It is the worst case window in which peers keep routing to a node that died without
// unregistering: lower it to shorten misrouting after a crash, at the cost of more
// keepalive traffic. The unit is seconds because that is what etcd's Grant takes.
//
// It panics on a non-positive value rather than letting etcd reject the Grant at
// startup with a message that does not mention this option.
func WithConfigEtcdLeaseTTLSecond(seconds int64) ConfigOptFunc {
	return func(config *config) {
		if seconds <= 0 {
			panic("grain: WithConfigEtcdLeaseTTLSecond needs a positive number of seconds, got " +
				strconv.FormatInt(seconds, 10))
		}
		config.etcdLeaseTTLSecond = seconds
	}
}

// WithConfigGrpcListenAddr sets the host:port the node's grpc server binds. Default
// ":0" — every interface, kernel-assigned port — because a fixed port would stop two
// nodes on one host from both starting.
//
// The address a node ADVERTISES to its peers is derived from this: a specific host is
// advertised as given, while a wildcard or empty host (":9000", "0.0.0.0:9000") is
// advertised as the top inner IP, falling back to 127.0.0.1 when the machine has no
// inner NIC. So binding one NIC also pins what peers dial. Port 0 stays usable with an
// explicit host ("10.0.0.7:0"): the kernel-assigned port is read back from the listener.
//
// Not sufficient for a node behind NAT or a container port mapping, where the reachable
// address is not one this process can see — that needs a separate advertise address,
// which the framework does not have yet.
func WithConfigGrpcListenAddr(addr string) ConfigOptFunc {
	return func(config *config) {
		if addr == "" {
			panic(`grain: WithConfigGrpcListenAddr needs a host:port, got "" (use ":0" for the default)`)
		}
		config.grpcListenAddr = addr
	}
}

// WithConfigGrpcDialOptions APPENDS dial options to the defaults.
//
// It used to replace the slice, which silently dropped the default
// insecure.NewCredentials() seeded in newConfig — so adding a single unrelated option
// made grpc.NewClient fail with "no transport security set", surfacing only as
// streamWriteActor logging and poisoning itself. If you need different credentials,
// pass grpc.WithTransportCredentials explicitly; the later option wins.
func WithConfigGrpcDialOptions(dialOptions ...grpc.DialOption) ConfigOptFunc {
	return func(config *config) {
		config.dialOptions = append(config.dialOptions, dialOptions...)
	}
}

// WithConfigCallDialOptions appends grpc call options.
//
// Deprecated: the name is wrong — these are CallOptions, nothing to do with dialing.
// Use WithConfigGrpcCallOptions.
func WithConfigCallDialOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return WithConfigGrpcCallOptions(callOptions...)
}

// WithConfigGrpcCallOptions appends grpc call options used for the outbound stream.
func WithConfigGrpcCallOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return func(config *config) {
		config.callOptions = append(config.callOptions, callOptions...)
	}
}

func WithConfigKind(kindName string, producer Producer, opts ...KindOptFunc) ConfigOptFunc {
	return func(config *config) {
		config.mustNotRunning()
		if kindName == defaultLocalKind ||
			kindName == defaultSystemKind ||
			kindName == defaultReplyKind ||
			kindName == defaultWriteStreamKind {
			panic("invalid kind name, please change")
		}
		if _, ok := config.kinds[kindName]; ok {
			panic("duplicate kind name " + kindName)
		}
		config.kinds[kindName] = tKind{producer: producer, opts: opts}
	}
}

// WithConfigLogger sets the logger this system derives all of its own loggers from —
// the system logger, the cluster provider's, and (via BaseActor.Logger) every actor's.
//
// This is the way to log without depending on global state. Left unset, the system reads
// slog.Default() when it builds its logger in Start(), which means InitLog (or any other
// slog.SetDefault) has to happen BEFORE Start() or none of the framework's output goes
// to it — an ordering rule nothing enforces and that is easy to get wrong, because the
// caller's own slog lines DO switch over while the framework's silently do not.
//
// Passing a logger here removes the ordering rule entirely: it is used regardless of what
// the global default is or when it changes. It does not touch slog.Default(), so the rest
// of the process is unaffected.
//
//	system := grain.NewSystem(name, ver, urls,
//	    grain.WithConfigLogger(grain.NewLogger("./game.log", slog.LevelInfo)))
//
// The system adds system=<addr> and node=<id> to whatever is passed; actors add
// actor=<ref> on top of that. A nil logger is ignored (the default applies).
func WithConfigLogger(l *slog.Logger) ConfigOptFunc {
	return func(config *config) {
		config.logger = l
	}
}

// WithConfigDeadLetter sets a system-wide handler for undeliverable messages
// (mailbox overflow or a send to a stopped actor). When unset, dead letters are
// logged at WARN. The handler runs on the sender's goroutine, so keep it fast
// and non-blocking.
func WithConfigDeadLetter(h DeadLetterHandler) ConfigOptFunc {
	return func(config *config) {
		config.deadLetterHandler = h
	}
}
