package actor

import (
	"github.com/chenxyzl/grain/actor/internal"
)

var messageDef = struct {
	//
	initialize *internal.Initialize
	poison     *internal.Poison
}{
	initialize: &internal.Initialize{},
	poison:     &internal.Poison{},
}
