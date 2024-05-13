package actor

import (
	"fmt"
	"google.golang.org/grpc"
	"net"
	"sync/atomic"
	"time"
)

const (
	defaultRequestTimeout = time.Second * 1
)

type KindProps struct {
	kind     string
	producer Producer
	onInit   func(ctx IActor)
}

type Config struct {
	running        int32
	name           string
	version        string
	remoteUrls     []string
	requestTimeout time.Duration
	dialOptions    []grpc.DialOption
	callOptions    []grpc.CallOption
	kinds          map[string]Producer
	addr           net.Addr
	state          NodeState
}

func NewConfig(clusterName string, version string, remoteUrls []string) *Config {
	return &Config{
		name:           clusterName,
		version:        version,
		remoteUrls:     remoteUrls,
		requestTimeout: defaultRequestTimeout,
		kinds:          make(map[string]Producer),
	}
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

// WithRequestTimeout set request timeout
func (x *Config) WithRequestTimeout(d time.Duration) *Config {
	x.requestTimeout = d
	return x
}

// WithGrpcDialOptions set grpc dialOptions
func (x *Config) WithGrpcDialOptions(dialOptions ...grpc.DialOption) *Config {
	x.mustNotRunning()
	x.dialOptions = dialOptions
	return x
}

// WithCallDialOptions set grpc dialOptions
func (x *Config) WithCallDialOptions(callOptions ...grpc.CallOption) *Config {
	x.mustNotRunning()
	x.callOptions = callOptions
	return x
}

// WithKind set kind
func (x *Config) WithKind(kindName string, producer Producer) *Config {
	x.mustNotRunning()
	if _, ok := x.kinds[kindName]; ok {
		panic("duplicate kind name " + kindName)
	}
	x.kinds[kindName] = producer
	return x
}
func (x *Config) GetMemberPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/%d", x.name, memberId)
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

// InitState after register
func (x *Config) InitState(addr string, nodeId uint64) NodeState {
	x.state = NodeState{NodeId: nodeId, Address: addr, Time: time.Now().Format(time.DateTime), Version: x.version, Kinds: x.GetKinds()}
	return x.state
}
