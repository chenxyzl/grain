package grain

import (
	"strings"
	"testing"
	"time"
)

// The three values below used to be file-level constants (provider_etcd.go's
// dialTimeoutTime / ttlTime and remote/stream_server.go's ":0"), so tuning any of them
// meant forking the framework. These tests pin both the defaults — changing one silently
// alters how long a dead node keeps receiving traffic — and the options.
func TestConfigDefaultsForEtcdAndGrpc(t *testing.T) {
	c := newConfig("cluster", "v1", []string{"127.0.0.1:2379"})
	if c.etcdDialTimeout != 10*time.Second {
		t.Errorf("etcdDialTimeout default = %v, want 10s", c.etcdDialTimeout)
	}
	if c.etcdLeaseTTLSecond != 10 {
		t.Errorf("etcdLeaseTTLSecond default = %d, want 10", c.etcdLeaseTTLSecond)
	}
	if c.grpcListenAddr != ":0" {
		t.Errorf("grpcListenAddr default = %q, want \":0\" (kernel-assigned port, so two "+
			"nodes on one host can both start)", c.grpcListenAddr)
	}
}

func TestConfigEtcdAndGrpcOptions(t *testing.T) {
	c := newConfig("cluster", "v1", nil,
		WithConfigEtcdDialTimeout(3*time.Second),
		WithConfigEtcdLeaseTTLSecond(4),
		WithConfigGrpcListenAddr("10.0.0.7:9000"),
	)
	if c.etcdDialTimeout != 3*time.Second {
		t.Errorf("etcdDialTimeout = %v, want 3s", c.etcdDialTimeout)
	}
	if c.etcdLeaseTTLSecond != 4 {
		t.Errorf("etcdLeaseTTLSecond = %d, want 4", c.etcdLeaseTTLSecond)
	}
	if c.grpcListenAddr != "10.0.0.7:9000" {
		t.Errorf("grpcListenAddr = %q, want 10.0.0.7:9000", c.grpcListenAddr)
	}
}

// Each option rejects its degenerate value at config time. Letting them through would
// surface much later and much less clearly: a zero DialTimeout means "no timeout" to
// clientv3, so a wrong endpoint hangs forever instead of failing; a zero TTL is rejected
// by etcd's Grant with a message that never mentions the option; an empty listen address
// is not distinguishable from "unset".
func TestConfigRejectsDegenerateValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  ConfigOptFunc
		want string
	}{
		{"zero dial timeout", WithConfigEtcdDialTimeout(0), "WithConfigEtcdDialTimeout"},
		{"negative dial timeout", WithConfigEtcdDialTimeout(-time.Second), "WithConfigEtcdDialTimeout"},
		{"zero lease ttl", WithConfigEtcdLeaseTTLSecond(0), "WithConfigEtcdLeaseTTLSecond"},
		{"negative lease ttl", WithConfigEtcdLeaseTTLSecond(-5), "WithConfigEtcdLeaseTTLSecond"},
		{"empty listen addr", WithConfigGrpcListenAddr(""), "WithConfigGrpcListenAddr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("want a panic naming the option, got none")
				}
				if msg, ok := r.(string); !ok || !strings.Contains(msg, tc.want) {
					t.Errorf("panic must name %s so the fix is obvious, got %v", tc.want, r)
				}
			}()
			newConfig("cluster", "v1", nil, tc.opt)
		})
	}
}
