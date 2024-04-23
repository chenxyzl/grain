package actor

import (
	"fmt"
	"google.golang.org/grpc"
	"log/slog"
	"net"
	"time"
)

type Config struct {
	name           string
	requestTimeout time.Duration
	DialOptions    []grpc.DialOption
	CallOptions    []grpc.CallOption
	kinds          map[string]Producer
	running        bool
	addr           net.Addr
	remoteUrls     []string
	version        string
	state          NodeState
}

func NewConfig(clusterName string, version string, remoteUrls []string) *Config {
	return &Config{name: clusterName, kinds: make(map[string]Producer), remoteUrls: remoteUrls, version: version}
}
func (x *Config) WithRequestTimeout(d time.Duration) *Config {
	x.requestTimeout = d
	return x
}
func (x *Config) WithKind(kindName string, producer Producer) {
	if x.running {
		slog.Error("add kind to actor already running, kind:" + kindName)
		return
	}
	if _, ok := x.kinds[kindName]; ok {
		panic("duplicate kind name " + kindName)
	}
	x.kinds[kindName] = producer
}

func (x *Config) GetMemberPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/%d", x.name, memberId)
}
func (x *Config) GetRemoteUrls() []string {
	return x.remoteUrls
}
