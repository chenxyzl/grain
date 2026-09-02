package grain

import (
	"errors"
	"sync"
	"testing"

	"github.com/chenxyzl/grain/message"
	"google.golang.org/protobuf/proto"
)

// The shared singletons get marshalled concurrently on the remote send path (one write_stream
// actor per peer), and proto.Marshal writes the generated struct's sizeCache. Run under -race.
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

// Read side: a failed Ask hands the same pointer to every waiting caller.
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

// The two ErrCode values that escape into user code must match their own code, and only their
// own code, through errors.Is.
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
			if errors.Is(tc.err, message.CodeErr) {
				t.Errorf("%s must not match the generic CodeErr", tc.name)
			}
		})
	}
}
