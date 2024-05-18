package actor

import (
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net"
	"sync/atomic"
	"time"
)

type ConfigOptFunc func(*Config)

const (
	defaultRequestTimeout     = time.Second * 3
	defaultStopWaitTimeSecond = 3
	defaultLocalKind          = "local"
	defaultSystemKind         = "system"
	defaultReplyKind          = "reply"
	writeStreamNamePrefix     = "write_stream_"
	eventStreamName           = "event_stream"
)

type NodeState struct {
	NodeId  uint64
	Address string
	Version string
	Time    string
	Kinds   []string
}

type Config struct {
	running            int32
	name               string
	version            string
	remoteUrls         []string
	requestTimeout     time.Duration
	stopWaitTimeSecond int
	dialOptions        []grpc.DialOption
	callOptions        []grpc.CallOption
	kinds              map[string]Kind
	addr               net.Addr
	state              NodeState
}

func NewConfig(clusterName string, version string, remoteUrls []string, opts ...ConfigOptFunc) *Config {
	config := &Config{
		name:               clusterName,
		version:            version,
		remoteUrls:         remoteUrls,
		requestTimeout:     defaultRequestTimeout,
		stopWaitTimeSecond: defaultStopWaitTimeSecond,
		kinds:              make(map[string]Kind),
		dialOptions:        []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	}
	for _, f := range opts {
		f(config)
	}
	return config
}

// markRunning ...
func (x *Config) markRunning() {
	if !atomic.CompareAndSwapInt32(&x.running, 0, 1) {
		panic("already running")
	}
}

// mustNotRunning ...
func (x *Config) mustNotRunning() {
	if atomic.LoadInt32(&x.running) != 0 {
		panic("already running")
	}
}

func WithRequestTimeout(d time.Duration) ConfigOptFunc {
	return func(config *Config) {
		config.requestTimeout = d
	}
}

func WithStopWaitTimeSecond(t int) ConfigOptFunc {
	return func(config *Config) {
		config.stopWaitTimeSecond = t
	}
}

func WithGrpcDialOptions(dialOptions ...grpc.DialOption) ConfigOptFunc {
	return func(config *Config) {
		config.dialOptions = dialOptions
	}
}

func WithCallDialOptions(callOptions ...grpc.CallOption) ConfigOptFunc {
	return func(config *Config) {
		config.callOptions = callOptions
	}
}

func WithKind(kindName string, producer Producer, opts ...KindOptFunc) ConfigOptFunc {
	return func(config *Config) {
		config.mustNotRunning()
		if kindName == defaultLocalKind ||
			kindName == defaultSystemKind ||
			kindName == defaultReplyKind {
			panic("invalid kind name, please change")
		}
		if _, ok := config.kinds[kindName]; ok {
			panic("duplicate kind name " + kindName)
		}
		config.kinds[kindName] = Kind{producer: producer, opts: opts}
	}
}

func (x *Config) GetMemberPrefix() string {
	return fmt.Sprintf("/%v/member/", x.name)
}
func (x *Config) GetMemberPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/%d", x.name, memberId)
}
func (x *Config) GetEventStreamPrefix() string {
	return fmt.Sprintf("/%v/%v/", x.name, eventStreamName)
}
func (x *Config) GetRemoteActorKind(ref *ActorRef) string {
	return fmt.Sprintf("/%v/remote/%v", x.name, ref.GetXPath())
}

// GetRemoteUrls ...
func (x *Config) GetRemoteUrls() []string {
	return x.remoteUrls
}

// GetKinds get all kinds
func (x *Config) GetKinds() []string {
	kinds := make([]string, 0, len(x.kinds))
	for kind := range x.kinds {
		kinds = append(kinds, kind)
	}
	return kinds
}

// init after register
func (x *Config) init(addr string, nodeId uint64) NodeState {
	x.state = NodeState{NodeId: nodeId, Address: addr, Time: time.Now().Format(time.DateTime), Version: x.version, Kinds: x.GetKinds()}
	return x.state
}
