package actor

import "time"

type Config struct {
	name            string
	clusterProvider ClusterProvider
	requestTimeout  time.Duration
	props           *Props
}

func NewConfig(clusterName string, clusterProvider ClusterProvider) *Config {
	return &Config{name: clusterName, clusterProvider: clusterProvider}
}
func (x *Config) WithRequestTimeout(d time.Duration) *Config {
	x.requestTimeout = d
	return x
}
