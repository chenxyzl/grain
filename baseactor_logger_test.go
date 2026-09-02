package grain

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestActorLoggerInheritsSystemAttrs pins that Logger() derives from the SYSTEM logger, not
// slog.Default(): an actor ref alone is the same string on whichever node holds the grain.
func TestActorLoggerInheritsSystemAttrs(t *testing.T) {
	var buf bytes.Buffer
	sys := newFakeSys()
	// mirror what system.init() builds: the system logger already carries system+node
	sys.logger = slog.New(slog.NewTextHandler(&buf, nil)).With("system", "1.2.3.4:9000", "node", 7)

	act := &replier{}
	p := newTestProcessor(sys, act, 8)
	p.init()

	act.Logger().Info("hello")

	line := buf.String()
	for _, want := range []string{"system=1.2.3.4:9000", "node=7", "actor="} {
		if !strings.Contains(line, want) {
			t.Errorf("actor log line is missing %q: %s", want, line)
		}
	}
}

// Cached after the first build: a second call must not rebuild it, nor re-append actor.
func TestActorLoggerIsCached(t *testing.T) {
	sys := newFakeSys()
	act := &replier{}
	p := newTestProcessor(sys, act, 8)
	p.init()

	if first, second := act.Logger(), act.Logger(); first != second {
		t.Error("Logger() rebuilt the logger on the second call")
	}
}
