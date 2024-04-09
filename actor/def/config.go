package def

import (
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"log/slog"
	"net"
	"time"
)

type Config struct {
	name           string
	requestTimeout time.Duration
	kinds          map[string]*tKind
	running        bool
	clusterUrl     []string
	addr           net.Addr
}

func NewConfig(clusterName string, clusterUrl []string) *Config {
	return &Config{name: clusterName, kinds: make(map[string]*tKind), clusterUrl: clusterUrl}
}
func (x *Config) WithRequestTimeout(d time.Duration) *Config {
	x.requestTimeout = d
	return x
}
func (x *Config) WithKind(kindName string, producer func(*actor.System) actor.IActor) {
	if x.running {
		slog.Error("add kind to actor already running, kind:" + kindName)
		return
	}
	if _, ok := x.kinds[kindName]; ok {
		panic("duplicate kind name " + kindName)
	}
	x.kinds[kindName] = &tKind{kind: kindName, producer: producer}
}

func (x *Config) GetClusterUrl() []string {
	return x.clusterUrl
}
func (x *Config) GetMemberPath(memberId uint64) string {
	return fmt.Sprintf("/%v/member/%d", x.name, memberId)
}
func (x *Config) BindLocalAddr(addr net.Addr) {
	x.addr = addr
}
