package message

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestCustomProto(t *testing.T) {
	test := &BroadcastPublishProtoWrapper{&Initialize{}}
	v1 := test.ProtoReflect().Descriptor().FullName()
	fmt.Println(v1)
	v2 := proto.MessageName(test)
	fmt.Println(v2)
}

// errors.Is must match on the code and ignore Des, so a caller never has to write
// err.Code == int32(...) again.
func TestErrCodeIs(t *testing.T) {
	err := WithErrCode(CodeActorNotFound, "actor not found")
	if !errors.Is(err, CodeActorNotFound) {
		t.Fatal("want match on the same code")
	}
	if errors.Is(err, CodeAskNotRunning) {
		t.Fatal("want no match on a different code")
	}
	// Des must not take part in the comparison: WithErrCode with a different
	// description is still the same failure.
	if !errors.Is(err, WithErrCode(CodeActorNotFound, "totally different words")) {
		t.Fatal("want match against another ErrCode with the same code")
	}
	// and it must survive wrapping
	if !errors.Is(fmt.Errorf("ask target %q: %w", "player/1", err), CodeActorNotFound) {
		t.Fatal("want match through a wrap")
	}
	// unrelated error types must not match
	if errors.Is(err, errors.New("boom")) {
		t.Fatal("want no match against a plain error")
	}
	// a nil *ErrCode is reachable through the error interface; it must report false,
	// not panic.
	var nilErr *ErrCode
	if nilErr.Is(CodeActorNotFound) {
		t.Fatal("want no match on a nil ErrCode")
	}
}

func TestCodeOf(t *testing.T) {
	if code, ok := CodeOf(WithErr("boom")); !ok || code != CodeErr {
		t.Fatalf("want CodeErr,true got %v,%v", code, ok)
	}
	if code, ok := CodeOf(fmt.Errorf("wrapped: %w", WithErrCode(CodeAskNotRunning))); !ok || code != CodeAskNotRunning {
		t.Fatalf("want CodeAskNotRunning,true got %v,%v", code, ok)
	}
	// a bare Code used as an error is also recognised
	if code, ok := CodeOf(CodeActorNotFound); !ok || code != CodeActorNotFound {
		t.Fatalf("want CodeActorNotFound,true got %v,%v", code, ok)
	}
	if _, ok := CodeOf(errors.New("boom")); ok {
		t.Fatal("want false for a non-ErrCode error")
	}
	if _, ok := CodeOf(nil); ok {
		t.Fatal("want false for nil")
	}
}
