package actor_test

import (
	"github.com/chenxyzl/grain/actor"
	"testing"
)

var _ actor.IActor = (*Actor)(nil)

type Actor struct {
	actor.BaseActor
}

func TestActor(t *testing.T) {
	var a = &Actor{}
	_ = a
}
