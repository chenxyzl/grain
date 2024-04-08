package actor

import (
	"log/slog"
	"time"
)

type Config struct {
	name            string
	clusterProvider ClusterProvider
	requestTimeout  time.Duration
	kinds           map[string]*tKind
	running         bool
}

func NewConfig(clusterName string, clusterProvider ClusterProvider) *Config {
	return &Config{name: clusterName, clusterProvider: clusterProvider, kinds: make(map[string]*tKind)}
}
func (x *Config) WithRequestTimeout(d time.Duration) *Config {
	x.requestTimeout = d
	return x
}
func (x *Config) WithKind(kindName string, producer func(*System) IActor) {
	if x.running {
		slog.Error("add kind to actor already running, kind:" + kindName)
		return
	}
	if _, ok := x.kinds[kindName]; ok {
		panic("duplicate kind name " + kindName)
	}
	x.kinds[kindName] = &tKind{kind: kindName, producer: producer}
}
