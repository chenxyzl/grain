package grain

import (
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ConfigOptFunc func(*config)

const (
	defaultAskTimeout         = time.Second * 3
	defaultStopWaitTimeSecond = 3
	//defaultEtcdDialTimeout bounds the initial clientv3 connect and the lease Revoke on shutdown.
	defaultEtcdDialTimeout = time.Second * 10
	//defaultEtcdLeaseTTLSecond: TTL of the lease this node's member key and every event-stream
	//subscription hang off — how long peers may route to a dead node. Whole seconds, as Grant takes.
	defaultEtcdLeaseTTLSecond = 10
	//defaultGrpcListenAddr: all interfaces, kernel-assigned port, so two nodes can share a host.
	defaultGrpcListenAddr = ":0"
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
	running            int32
	clusterName        string
	version            string
	clusterUrls        []string
	askTimeout         time.Duration
	stopWaitTimeSecond int
	etcdDialTimeout    time.Duration
	etcdLeaseTTLSecond int64
	grpcListenAddr     string
	dialOptions        []grpc.DialOption
	callOptions        []grpc.CallOption
	kinds              map[string]tKind
	state              tNodeState
	deadLetterHandler  DeadLetterHandler
	//logger, when non-nil, is what the system derives its loggers from instead of slog.Default().
	logger *slog.Logger
}

func newConfig(clusterName string, version string, clusterUrls []string, opts ...ConfigOptFunc) *config {
	conf := &config{
		clusterName:        clusterName,
		version:            version,
		clusterUrls:        clusterUrls,
		askTimeout:         defaultAskTimeout,
		stopWaitTimeSecond: defaultStopWaitTimeSecond,
		etcdDialTimeout:    defaultEtcdDialTimeout,
		etcdLeaseTTLSecond: defaultEtcdLeaseTTLSecond,
		grpcListenAddr:     defaultGrpcListenAddr,
		kinds:              make(map[string]tKind),
		dialOptions:        []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	}
	for _, f := range opts {
		f(conf)
	}
	return conf
}

// init after register
func (x *config) init(addr string, nodeId uint64) tNodeState {
	x.state = tNodeState{NodeId: nodeId, Address: addr, Time: time.Now().Format(time.DateTime), Version: x.version, Kinds: x.getKinds()}
	return x.state
}

func (x *config) markRunning() {
	if !atomic.CompareAndSwapInt32(&x.running, 0, 1) {
		panic("already running")
	}
}

func (x *config) mustNotRunning() {
	if atomic.LoadInt32(&x.running) != 0 {
		panic("already running")
	}
}

func (x *config) getKinds() []string {
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
func (x *config) getMemberExtDataPath(subKey string, memberId ...uint64) string {
	if len(memberId) > 0 && memberId[0] != 0 {
		return fmt.Sprintf("/%v/member_ext/%s/%d", x.clusterName, subKey, memberId[0])
	}
	return fmt.Sprintf("/%v/member_ext/%s", x.clusterName, subKey)
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
