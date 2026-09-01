package grain

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// newBufLogger returns a logger writing plain text into buf, tagged so a test can tell
// which of several loggers produced a line.
func newBufLogger(buf *bytes.Buffer, tag string) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil)).With("src", tag)
}

// setDefaultForTest swaps slog's process-wide default and restores it when the test ends.
// These tests exist precisely because the default is global state, so they must not leave
// it changed for whatever test runs next.
func setDefaultForTest(t *testing.T, l *slog.Logger) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(l)
}

// TestWithConfigLoggerWins: an explicitly configured logger must be used no matter what
// the slog default is, and must survive both tagging steps (Start and init). Those used
// to call slog.With(...), which read the global and would have discarded a configured
// logger entirely.
func TestWithConfigLoggerWins(t *testing.T) {
	var configured, global bytes.Buffer
	setDefaultForTest(t, newBufLogger(&global, "global"))

	sys := &system{config: newConfig("c", "v", nil,
		WithConfigLogger(newBufLogger(&configured, "configured")))}
	sys.logger = sys.config.logger // NewSystem

	// mirror the two tagging sites (system_life.go Start / init) without needing etcd
	sys.addr = "1.2.3.4:9000"
	sys.logger = sys.Logger().With("system", sys.addr)
	sys.Logger().Info("from start")
	sys.config.state.NodeId = 7
	sys.logger = sys.Logger().With("node", sys.config.state.NodeId)
	sys.Logger().Info("from init")

	if global.Len() != 0 {
		t.Errorf("a configured logger must not fall through to slog.Default(), got: %s", global.String())
	}
	got := configured.String()
	for _, want := range []string{"src=configured", "from start", "from init", "node=7"} {
		if !strings.Contains(got, want) {
			t.Errorf("configured logger output missing %q: %s", want, got)
		}
	}
	// "system" must appear once per line, not twice: init appends only the node id to the
	// logger Start() already tagged, rather than re-adding the address.
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if n := strings.Count(line, "system="); n > 1 {
			t.Errorf("system attr duplicated %d times in: %s", n, line)
		}
	}
}

// TestNoConfigLoggerResolvesDefaultLate is the ordering rule InitLog depends on: with no
// configured logger, the default is read at each rebuild, so a SetDefault issued after
// NewSystem but before Start() still reaches the framework.
func TestNoConfigLoggerResolvesDefaultLate(t *testing.T) {
	var early, late bytes.Buffer
	setDefaultForTest(t, newBufLogger(&early, "early"))

	sys := &system{config: newConfig("c", "v", nil)}
	sys.logger = sys.config.logger // NewSystem: stays nil, which is the mechanism

	// InitLog-equivalent, AFTER NewSystem
	setDefaultForTest(t, newBufLogger(&late, "late"))

	sys.addr = "1.2.3.4:9000"
	sys.logger = sys.Logger().With("system", sys.addr) // Start()
	sys.Logger().Info("after start")

	if !strings.Contains(late.String(), "after start") {
		t.Errorf("a SetDefault between NewSystem and Start must be picked up, late buf: %q early buf: %q",
			late.String(), early.String())
	}
}

// A nil logger must be ignored rather than stored — storing it would nil-panic on the
// first .With call.
func TestWithConfigLoggerNilIsIgnored(t *testing.T) {
	var global bytes.Buffer
	setDefaultForTest(t, newBufLogger(&global, "global"))

	sys := &system{config: newConfig("c", "v", nil, WithConfigLogger(nil))}
	sys.logger = sys.config.logger
	sys.Logger().Info("hello") // must not panic

	if !strings.Contains(global.String(), "hello") {
		t.Errorf("a nil configured logger must fall back to slog.Default(), got %q", global.String())
	}
}

// The whole point of item 13: actors inherit the configured logger too, so one option
// covers system, provider and actor output.
func TestActorLoggerInheritsConfiguredLogger(t *testing.T) {
	var configured bytes.Buffer
	setDefaultForTest(t, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) // must NOT be used

	sys := newFakeSys()
	sys.logger = newBufLogger(&configured, "configured").With("system", "1.2.3.4:9000", "node", 7)

	act := &replier{}
	p := newTestProcessor(sys, act, 8)
	p.init()
	act.Logger().Info("from the actor")

	got := configured.String()
	for _, want := range []string{"src=configured", "system=1.2.3.4:9000", "node=7", "actor=", "from the actor"} {
		if !strings.Contains(got, want) {
			t.Errorf("actor log line missing %q: %s", want, got)
		}
	}
}

// ForceStop logs through the system logger, and x.logger is now deliberately nil until
// Start() builds it (that is what makes the late slog.Default() resolution work). So the
// no-Start path has to go through Logger() rather than the field, or a ForceStop before
// Start — e.g. a config error in the caller's own startup sequence — nil-panics.
func TestForceStopBeforeStartDoesNotPanic(t *testing.T) {
	var global bytes.Buffer
	setDefaultForTest(t, newBufLogger(&global, "global"))

	sys := NewSystem("c", "v", nil).(*system)
	if sys.logger != nil {
		t.Fatal("with no WithConfigLogger, NewSystem must leave the logger unbuilt so " +
			"Logger() can resolve slog.Default() late")
	}
	sys.ForceStop(nil) // must not panic

	if !strings.Contains(global.String(), "forceStop") {
		t.Errorf("ForceStop should have logged through the process default, got %q", global.String())
	}
}
