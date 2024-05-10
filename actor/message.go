package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
)

var messageDef = struct {
	//
	streamClosed         *internal.StreamClosed
	poison               *internal.Poison
	poisonIgnoreRegistry *internal.Poison
}{
	//
	streamClosed:         &internal.StreamClosed{},
	poison:               &internal.Poison{},
	poisonIgnoreRegistry: &internal.Poison{IgnoreRegistry: true},
}
