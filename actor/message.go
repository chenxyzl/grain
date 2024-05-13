package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
)

var messageDef = struct {
	//
	streamClosed *internal.StreamClosed
	initialize   *internal.Initialize
	poison       *internal.Poison
}{
	//
	streamClosed: &internal.StreamClosed{},
	initialize:   &internal.Initialize{},
	poison:       &internal.Poison{},
}
