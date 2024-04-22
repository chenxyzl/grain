package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
)

var Message = struct {
	//
	streamClosed *internal.StreamClosed
	start        *internal.Start
	stop         *internal.Stop
	poison       *internal.Poison
}{
	//
	streamClosed: &internal.StreamClosed{},
	start:        &internal.Start{},
	stop:         &internal.Stop{},
	poison:       &internal.Poison{},
}
