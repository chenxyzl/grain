package grain

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/chenxyzl/grain/al/safemap"
	"github.com/chenxyzl/grain/uuid"
)

func TestTemp(t *testing.T) {
	_ = uuid.Init(1)
	lookup := safemap.NewIntC[int, string]()
	for i := 0; i < 1000; i++ {
		time.Sleep(time.Millisecond * 2)
		v := uuid.Generate()
		ac := newDirectActorRef("local", strconv.Itoa(int(v)), "", nil)
		lookup.Set(int(v), ac.GetId())
	}
	fmt.Println(":1")
}

func TestTemp1(t *testing.T) {
	_ = uuid.Init(1)
	lookup := safemap.NewIntC[int, string]()
	for i := 0; i < 1000; i++ {
		//time.Sleep(time.Millisecond * 2)
		v := uuid.Generate()
		ac := newDirectActorRef("local", strconv.Itoa(int(v)), "", nil)
		lookup.Set(int(v), ac.GetId())
	}
	fmt.Println(":1")
}

func TestActorIdStr(t *testing.T) {
	id1 := newDirectActorRef("player", "123", "[::]:39318", nil)
	id2 := newClusterActorRef("home", "456", nil)
	if id1.GetId() != defaultActDirect+"/player/123" {
		t.Error("id1 should be kinds/player/123")
	}
	if id1.GetKind() != "player" {
		t.Error("id1 kind should be player")
	}
	if id1.GetName() != "123" {
		t.Error("id1 name should be 123")
	}
	if id1.GetDirectAddr() != "[::]:39318" {
		t.Error("id1 addr should be [::]:39318")
	}
	if id2.GetKind() != "home" {
		t.Error("id2 kind should be home")
	}
	if id2.GetName() != "456" {
		t.Error("id2 name should be 456")
	}
	if id2.GetDirectAddr() != "" {
		t.Error("id2 addr should be empty")
	}
}
