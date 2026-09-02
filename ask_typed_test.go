package grain

import (
	"strings"
	"testing"
	"time"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// These tests pin the typed Ask's reply-decoding contract: the *ErrCode and *Poison sentinels
// are matched before T, so they surface as errors for every T, including proto.Message.

// replier answers each Subscribe with whatever reply() produces.
type replier struct {
	BaseActor
	reply func() proto.Message
}

func (a *replier) Started() {}
func (a *replier) PreStop() {}
func (a *replier) Receive(ctx Context) {
	if _, ok := ctx.Message().(*message.Subscribe); ok {
		ctx.Reply(a.reply())
	}
}

func newReplierProcessor(t *testing.T, reply func() proto.Message) (*fakeSys, iProcess) {
	t.Helper()
	sys := newFakeSys()
	p := newTestProcessor(sys, &replier{reply: reply}, 8)
	p.init()
	return sys, p
}

func TestAskErrCodeReplyIsReturnedAsError(t *testing.T) {
	_, p := newReplierProcessor(t, func() proto.Message { return message.WithErr("boom") })

	v, err := NoReentryAsk[*message.Unsubscribe](p.self(), &message.Subscribe{EventName: "x"})
	if err == nil {
		t.Fatalf("an ErrCode reply must surface as an error, got value=%v err=nil", v)
	}
	if v != nil {
		t.Errorf("value must be the zero T on failure, got %v", v)
	}
	if !strings.Contains(err.Des, "boom") {
		t.Errorf("original ErrCode should be passed through, got %q", err.Des)
	}
}

// With T = proto.Message, a `case T`-first ordering would match *ErrCode and report success.
func TestAskErrCodeNotSwallowedByInterfaceT(t *testing.T) {
	_, p := newReplierProcessor(t, func() proto.Message { return message.WithErr("boom") })

	v, err := NoReentryAsk[proto.Message](p.self(), &message.Subscribe{EventName: "x"})
	if err == nil {
		t.Fatalf("Ask[proto.Message] must not report an ErrCode reply as success, got value=%v", v)
	}
	if !strings.Contains(err.Des, "boom") {
		t.Errorf("expected the ErrCode to pass through, got %q", err.Des)
	}
}

// Shutdown path: wakePendingAsks pushes poison into waiting channels, so Ask returns at once.
func TestAskPoisonReplyIsReturnedAsError(t *testing.T) {
	_, p := newReplierProcessor(t, func() proto.Message { return msgPoison })

	v, err := NoReentryAsk[*message.Unsubscribe](p.self(), &message.Subscribe{EventName: "x"})
	if err == nil {
		t.Fatalf("a Poison reply must surface as an error, got value=%v err=nil", v)
	}
	if !strings.Contains(err.Des, "poison") {
		t.Errorf("expected a poisoned-reply error, got %q", err.Des)
	}
}

func TestAskHappyPathReturnsTypedReply(t *testing.T) {
	_, p := newReplierProcessor(t, func() proto.Message {
		return &message.Unsubscribe{EventName: "pong"}
	})

	v, err := NoReentryAsk[*message.Unsubscribe](p.self(), &message.Subscribe{EventName: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil || v.EventName != "pong" {
		t.Errorf("expected the typed reply, got %v", v)
	}
}

// Also pins the diagnostic: the wanted name comes from proto.MessageName on the zero T, which
// only yields a real name because T is concrete (a nil proto.Message interface returns "").
func TestAskTypeMismatchNamesBothTypes(t *testing.T) {
	_, p := newReplierProcessor(t, func() proto.Message {
		return &message.Unsubscribe{EventName: "wrong type"}
	})

	v, err := NoReentryAsk[*message.Subscribe](p.self(), &message.Subscribe{EventName: "x"})
	if err == nil {
		t.Fatalf("a reply of the wrong type must be an error, got value=%v", v)
	}
	if !strings.Contains(err.Des, "message.Subscribe") || !strings.Contains(err.Des, "message.Unsubscribe") {
		t.Errorf("error should name the wanted and the received type, got %q", err.Des)
	}
}

// The nil-target guard lives in askImpl, so NoReentryAsk(nil, ...) errors instead of panicking.
func TestNoReentryAskNilTarget(t *testing.T) {
	v, err := NoReentryAsk[*message.Unsubscribe](nil, &message.Subscribe{EventName: "x"})
	if err == nil {
		t.Fatalf("a nil target must be an error, got value=%v", v)
	}
	if !strings.Contains(err.Des, "target is nil") {
		t.Errorf("expected a nil-target error, got %q", err.Des)
	}
}

// selfAskErrActor self-asks and replies with an ErrCode, via the reentrant BaseActor.Ask[T].
type selfAskErrActor struct {
	BaseActor
	out chan askOutcome
}

type askOutcome struct {
	nilValue bool
	err      *message.ErrCode
}

func (a *selfAskErrActor) Started() {}
func (a *selfAskErrActor) PreStop() {}
func (a *selfAskErrActor) Receive(ctx Context) {
	m, ok := ctx.Message().(*message.Subscribe)
	if !ok {
		return
	}
	switch m.EventName {
	case "go":
		v, err := a.Ask[*message.Unsubscribe](a.Self(), &message.Subscribe{EventName: "req"})
		a.out <- askOutcome{nilValue: v == nil, err: err}
	case "req":
		ctx.Reply(message.WithErr("boom"))
	}
}

func TestBaseActorAskErrCodeReplyIsReturnedAsError(t *testing.T) {
	sys := newFakeSys()
	act := &selfAskErrActor{out: make(chan askOutcome, 1)}
	p := newTestProcessor(sys, act, 8)
	p.init()
	p.send(newContext(p.self(), nil, &message.Subscribe{EventName: "go"}, sys.nextSnId(), sys))

	select {
	case got := <-act.out:
		if got.err == nil {
			t.Fatalf("BaseActor.Ask must surface an ErrCode reply as an error")
		}
		if !got.nilValue {
			t.Errorf("value must be the zero T on failure")
		}
		if !strings.Contains(got.err.Des, "boom") {
			t.Errorf("expected the ErrCode to pass through, got %q", got.err.Des)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for the self-ask to return")
	}
}
