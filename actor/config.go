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
	//actor kind
	defaultLocalKind       = "local"
	defaultSystemKind      = "system"
	defaultReplyKind       = "reply"
	defaultWriteStreamKind = "write_stream"
	//actor name
	eventStreamName = "event_stream"
)

type tNodeState struct {
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
	state              tNodeState
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

// init after register
func (x *Config) init(addr string, nodeId uint64) tNodeState {
	x.state = tNodeState{NodeId: nodeId, Address: addr, Time: time.Now().Format(time.DateTime), Version: x.version, Kinds: x.GetKinds()}
	return x.state
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

// GetKinds get all kinds
func (x *Config) GetKinds() []string {
	kinds := make([]string, 0, len(x.kinds))
	for kind := range x.kinds {
		kinds = append(kinds, kind)
	}
	return kinds
}

func (x *Config) GetMemberPrefix() string {
	return fmt.Sprintf("/%v/member/", x.name)
}
func (x *Config) GetMemberPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/%d", x.name, memberId)
}
func (x *Config) GetMemberExtDataPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/ext/%d", x.name, memberId)
}
func (x *Config) GetEventStreamPrefix() string {
	return fmt.Sprintf("/%v/%v/", x.name, eventStreamName)
}
func (x *Config) GetRemoteActorKind(ref *ActorRef) string {
	return fmt.Sprintf("/%v/remote/%v", x.name, ref.GetXPath())
}
func (x *Config) GetRemoteUrls() []string {
	return x.remoteUrls
}
