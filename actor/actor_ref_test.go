package actor

import (
	"fmt"
	"github.com/chenxyzl/grain/actor/uuid"
	"github.com/chenxyzl/grain/utils/al/safemap"
	"strconv"
	"testing"
	"time"
)

func TestTemp(t *testing.T) {
	uuid.Init(1)
	lookup := safemap.NewIntC[int, string]()
	for i := 0; i < 1000; i++ {
		time.Sleep(time.Millisecond * 2)
		v := uuid.Generate()
		ac := newActorRefWithKind("", "local", strconv.Itoa(int(v)))
		lookup.Set(int(v), ac.GetFullIdentifier())
	}
	fmt.Println(":1")
}

func TestTemp1(t *testing.T) {
	uuid.Init(1)
	lookup := safemap.NewIntC[int, string]()
	for i := 0; i < 1000; i++ {
		//time.Sleep(time.Millisecond * 2)
		v := uuid.Generate()
		ac := newActorRefWithKind("", "local", strconv.Itoa(int(v)))
		lookup.Set(int(v), ac.GetFullIdentifier())
	}
	fmt.Println(":1")
}