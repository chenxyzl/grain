package examples

import (
	"fmt"
	"github.com/chenxyzl/grain/actor"
	"github.com/chenxyzl/grain/actor/iface"
	"testing"
)

func TestString(t *testing.T) {
	v := iface.NewActorRef(&actor.Address{Id: "xxx"})
	fmt.Printf("%v\n", v)
}
