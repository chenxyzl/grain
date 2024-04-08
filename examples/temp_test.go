package examples

import (
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"testing"
)

func TestString(t *testing.T) {
	v := actor.NewActorRef(&actor.Address{Id: "xxx"})
	fmt.Printf("%v\n", v)
}
