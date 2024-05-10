package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
)

var messageDef = struct {
	//
	streamClosed          *internal.StreamClosed
	poison                *internal.Poison
	poisonWithoutRegistry *internal.Poison
}{
	//
	streamClosed:          &internal.StreamClosed{},
	poison:                &internal.Poison{WithRegistry: true},
	poisonWithoutRegistry: &internal.Poison{WithRegistry: false},
}
