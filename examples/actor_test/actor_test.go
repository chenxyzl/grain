package actor_test

import (
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"testing"
)

var _ actor.IActor = (*Actor)(nil)

type Actor struct {
	actor.BaseActor
}

func (a *Actor) Started() error {
	fmt.Printf("started actor\n")
	return nil
}

func (a *Actor) PreStop() error {
	fmt.Printf("preStop actor\n")
	return nil
}

func (a *Actor) Receive(ctx actor.IContext) {
	fmt.Printf("preStop actor\n")
}

func TestActor(t *testing.T) {
	var a = &Actor{}
	a.Started()
	a.PreStop()
	_ = a
}
