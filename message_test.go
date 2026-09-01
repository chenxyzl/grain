package grain

import (
	"errors"
	"sync"
	"testing"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// The real path: stream_write.go does proto.Marshal(msg) for a remote send. With two
// remote peers there are two write_stream actors, each with its own goroutine, so the
// SAME shared singleton can be marshalled concurrently. proto.Marshal writes the
// generated struct's sizeCache — is that safe? Run under -race.
func TestConcurrentMarshalOfSharedSingletons(t *testing.T) {
	for name, msg := range map[string]proto.Message{
		"errActorNotFound": errActorNotFound,
		"errAskNotRunning": errAskNotRunning,
		"msgPoison":        msgPoison,
		"msgInitialize":    msgInitialize,
	} {
		t.Run(name, func(t *testing.T) {
			var wg sync.WaitGroup
			for range 16 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 2000 {
						if _, err := proto.Marshal(msg); err != nil {
							t.Error(err)
							return
						}
					}
				}()
			}
			wg.Wait()
		})
	}
}

// And the read side: a failed Ask hands the same pointer to every waiting caller, who
// will at minimum read Code/Des (and Error()).
func TestConcurrentReadOfSharedErrCode(t *testing.T) {
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5000 {
				_ = errActorNotFound.Error()
				_ = errActorNotFound.GetCode()
				_ = errActorNotFound.GetDes()
			}
		}()
	}
	wg.Wait()
}

// The two ErrCode values that escape into user code must be matchable by code through
// errors.Is. Before ErrCode.Is existed, the only way to tell "actor not found" from
// "timeout" was err.Code == int32(message.CodeActorNotFound), so this pins the
// ergonomic contract as well as the codes themselves.
func TestFrameworkSentinelsMatchTheirCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code message.Code
	}{
		{"errActorNotFound", errActorNotFound, message.CodeActorNotFound},
		{"errAskNotRunning", errAskNotRunning, message.CodeAskNotRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.code) {
				t.Errorf("errors.Is(%s, %v) = false, want true", tc.name, tc.code)
			}
			// and it must not match a code it is not
			if errors.Is(tc.err, message.CodeErr) {
				t.Errorf("%s must not match the generic CodeErr", tc.name)
			}
		})
	}
}
