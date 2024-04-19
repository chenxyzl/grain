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
	kinds          map[string]ProducerFunc
	running        bool
	addr           net.Addr
	remoteUrls     []string
}

func NewConfig(clusterName string, remoteUrls []string) *Config {
	return &Config{name: clusterName, kinds: make(map[string]ProducerFunc), remoteUrls: remoteUrls}
}
func (x *Config) WithRequestTimeout(d time.Duration) *Config {
	x.requestTimeout = d
	return x
}
func (x *Config) WithKind(kindName string, producer ProducerFunc) {
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
