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

// WithConfigEtcdDialTimeout bounds the initial etcd connect and the lease Revoke on shutdown.
// Default 10s. Panics on <= 0: clientv3 reads a zero DialTimeout as "no timeout", so a wrong
// endpoint hangs instead of failing at startup.
func WithConfigEtcdDialTimeout(d time.Duration) ConfigOptFunc {
	return func(config *config) {
		if d <= 0 {
			panic("grain: WithConfigEtcdDialTimeout needs a positive duration, got " + d.String())
		}
		config.etcdDialTimeout = d
	}
}

// WithConfigEtcdLeaseTTLSecond sets the TTL of the etcd lease this node's member key and
// event-stream subscriptions hang off, i.e. how long peers may keep routing to a node that died
// without unregistering. Default 10, unit seconds (what etcd's Grant takes). Panics on <= 0.
func WithConfigEtcdLeaseTTLSecond(seconds int64) ConfigOptFunc {
	return func(config *config) {
		if seconds <= 0 {
			panic("grain: WithConfigEtcdLeaseTTLSecond needs a positive number of seconds, got " +
				strconv.FormatInt(seconds, 10))
		}
		config.etcdLeaseTTLSecond = seconds
	}
}

// WithConfigGrpcListenAddr sets the host:port the node's grpc server binds. Default ":0" —
// every interface, kernel-assigned port — so two nodes on one host can both start. Panics on "".
//
// The address ADVERTISED to peers derives from it: a specific host as given, a wildcard or empty
// one as the top inner IP (else 127.0.0.1), port 0 read back from the listener — so binding one
// NIC pins what peers dial. Wrong behind NAT/port mapping; no separate advertise address yet.
func WithConfigGrpcListenAddr(addr string) ConfigOptFunc {
	return func(config *config) {
		if addr == "" {
			panic(`grain: WithConfigGrpcListenAddr needs a host:port, got "" (use ":0" for the default)`)
		}
		config.grpcListenAddr = addr
	}
}

// WithConfigGrpcDialOptions APPENDS dial options to the defaults; replacing them would drop
// newConfig's insecure.NewCredentials(). For other credentials pass yours — the later one wins.
func WithConfigGrpcDialOptions(dialOptions ...grpc.DialOption) ConfigOptFunc {
	return func(config *config) {
		config.dialOptions = append(config.dialOptions, dialOptions...)
	}
}

// WithConfigCallDialOptions appends grpc call options.
//
// Deprecated: these are CallOptions, nothing to do with dialing. Use WithConfigGrpcCallOptions.
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

// WithConfigLogger sets the logger this system derives all of its own loggers from: the system
// logger (tagged system=<addr>, node=<id>), the cluster provider's and, via BaseActor.Logger,
// every actor's (actor=<ref>). A nil logger is ignored. slog.Default() is left untouched.
//
// Unset, the system reads slog.Default() when Start() builds its logger, so slog.SetDefault
// must happen BEFORE Start() — an ordering rule nothing enforces, and this option removes.
func WithConfigLogger(l *slog.Logger) ConfigOptFunc {
	return func(config *config) {
		config.logger = l
	}
}

// WithConfigDeadLetter sets a system-wide handler for undeliverable messages (mailbox overflow,
// send to a stopped actor); unset they are logged at WARN. Runs on the sender's goroutine.
func WithConfigDeadLetter(h DeadLetterHandler) ConfigOptFunc {
	return func(config *config) {
		config.deadLetterHandler = h
	}
}
