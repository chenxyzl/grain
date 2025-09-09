package grain

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ConfigOptFunc func(*config)

const (
	defaultAskTimeout         = time.Second * 3
	defaultStopWaitTimeSecond = 3
	//actor type
	defaultActDirect  = "direct"
	defaultActCluster = "cluster"

	//actor kind
	defaultLocalKind       = "local"
	defaultSystemKind      = "system"
	defaultReplyKind       = "reply"
	defaultWriteStreamKind = "write_stream"
	//actor name
	eventStreamWatchName = "event_stream"
)

type tNodeState struct {
	NodeId  uint64
	Address string
	Version string
	Time    string
	Kinds   []string
}

type config struct {
	running              int32
	clusterName          string
	version              string
	clusterUrls          []string
	askTimeout           time.Duration
	stopWaitTimeSecond   int
	dialOptions          []grpc.DialOption
	callOptions          []grpc.CallOption
	kinds                map[string]tKind
	addr                 net.Addr
	state                tNodeState
	localProviderVersion int64 //
}

func newConfig(clusterName string, version string, clusterUrls []string, opts ...ConfigOptFunc) *config {
	config := &config{
		clusterName:        clusterName,
		version:            version,
		clusterUrls:        clusterUrls,
		askTimeout:         defaultAskTimeout,
		stopWaitTimeSecond: defaultStopWaitTimeSecond,
		kinds:              make(map[string]tKind),
		dialOptions:        []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	}
	for _, f := range opts {
		f(config)
	}
	return config
}

// init after register
func (x *config) init(addr string, nodeId uint64) tNodeState {
	x.state = tNodeState{NodeId: nodeId, Address: addr, Time: time.Now().Format(time.DateTime), Version: x.version, Kinds: x.GetKinds()}
	return x.state
}

// markRunning ...
func (x *config) markRunning() {
	if !atomic.CompareAndSwapInt32(&x.running, 0, 1) {
		panic("already running")
	}
}

// mustNotRunning ...
func (x *config) mustNotRunning() {
	if atomic.LoadInt32(&x.running) != 0 {
		panic("already running")
	}
}

// GetKinds get all kinds
func (x *config) GetKinds() []string {
	kinds := make([]string, 0, len(x.kinds))
	for kind := range x.kinds {
		kinds = append(kinds, kind)
	}
	return kinds
}

func (x *config) getMemberPrefix() string {
	return fmt.Sprintf("/%v/member/", x.clusterName)
}
func (x *config) getMemberPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/%d", x.clusterName, memberId)
}
func (x *config) getMemberExtDataPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member_ext/%d", x.clusterName, memberId)
}
func (x *config) getEventStreamWatchPath() string {
	return fmt.Sprintf("/%v/%v/", x.clusterName, eventStreamWatchName)
}
func (x *config) getActorRegisterName(ref ActorRef) string {
	return fmt.Sprintf("/%v/%v", x.clusterName, ref.GetId())
}
func (x *config) getClusterUrls() []string {
	return x.clusterUrls
}
