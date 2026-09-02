package grain

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// newBufLogger writes plain text into buf, tagged so a test can tell the loggers apart.
func newBufLogger(buf *bytes.Buffer, tag string) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil)).With("src", tag)
}

// setDefaultForTest swaps slog's process-wide default and restores it when the test ends.
func setDefaultForTest(t *testing.T, l *slog.Logger) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(l)
}

// TestWithConfigLoggerWins: a configured logger beats the slog default, through both tagging steps.
func TestWithConfigLoggerWins(t *testing.T) {
	var configured, global bytes.Buffer
	setDefaultForTest(t, newBufLogger(&global, "global"))

	sys := &system{config: newConfig("c", "v", nil,
		WithConfigLogger(newBufLogger(&configured, "configured")))}
	sys.logger.Store(sys.config.logger) // NewSystem

	// mirror the two tagging sites (system_life.go Start / init) without etcd
	sys.addr = "1.2.3.4:9000"
	sys.logger.Store(sys.Logger().With("system", sys.addr))
	sys.Logger().Info("from start")
	sys.config.state.NodeId = 7
	sys.logger.Store(sys.Logger().With("node", sys.config.state.NodeId))
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
	// "system" once per line, not twice: init appends only the node id to Start()'s logger.
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if n := strings.Count(line, "system="); n > 1 {
			t.Errorf("system attr duplicated %d times in: %s", n, line)
		}
	}
}

// TestNoConfigLoggerResolvesDefaultLate pins the ordering rule InitLog depends on: with no
// configured logger the default is read at each rebuild, so a SetDefault after NewSystem lands.
func TestNoConfigLoggerResolvesDefaultLate(t *testing.T) {
	var early, late bytes.Buffer
	setDefaultForTest(t, newBufLogger(&early, "early"))

	sys := &system{config: newConfig("c", "v", nil)} // NewSystem: nil logger is the mechanism

	setDefaultForTest(t, newBufLogger(&late, "late"))

	sys.addr = "1.2.3.4:9000"
	sys.logger.Store(sys.Logger().With("system", sys.addr)) // Start()
	sys.Logger().Info("after start")

	if !strings.Contains(late.String(), "after start") {
		t.Errorf("a SetDefault between NewSystem and Start must be picked up, late buf: %q early buf: %q",
			late.String(), early.String())
	}
}

// A nil logger must be ignored, not stored — storing it would nil-panic on the first .With.
func TestWithConfigLoggerNilIsIgnored(t *testing.T) {
	var global bytes.Buffer
	setDefaultForTest(t, newBufLogger(&global, "global"))

	sys := &system{config: newConfig("c", "v", nil, WithConfigLogger(nil))}
	sys.Logger().Info("hello") // must not panic

	if !strings.Contains(global.String(), "hello") {
		t.Errorf("a nil configured logger must fall back to slog.Default(), got %q", global.String())
	}
}

// Actors inherit the configured logger too, so one option covers system, provider and actor.
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

// x.logger stays nil until Start() builds it (that is what makes late slog.Default() resolution
// work), so ForceStop must log via Logger(): reading the field would nil-panic before Start().
func TestForceStopBeforeStartDoesNotPanic(t *testing.T) {
	var global bytes.Buffer
	setDefaultForTest(t, newBufLogger(&global, "global"))

	sys := NewSystem("c", "v", nil).(*system)
	if sys.logger.Load() != nil {
		t.Fatal("with no WithConfigLogger, NewSystem must leave the logger unbuilt so " +
			"Logger() can resolve slog.Default() late")
	}
	sys.ForceStop(nil) // must not panic

	if !strings.Contains(global.String(), "forceStop") {
		t.Errorf("ForceStop should have logged through the process default, got %q", global.String())
	}
}

// TestSystemLoggerIsRaceFree: Start() and init() build x.logger while grpc is already accepting,
// so RecvEnvelope's Logger() read races those writes. Run under -race.
func TestSystemLoggerIsRaceFree(t *testing.T) {
	sys := &system{config: newConfig("c", "v", nil)}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 { // readers: grpc handler goroutines
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = sys.Logger()
				}
			}
		}()
	}
	for range 200 { // writers: Start() then init()
		sys.logger.Store(sys.Logger().With("system", "1.2.3.4:9000"))
	}
	close(stop)
	wg.Wait()
}
