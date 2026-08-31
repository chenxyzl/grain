package grain

import (
	"sync"
	"testing"

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
		"poison":           poison,
		"initialize":       initialize,
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
