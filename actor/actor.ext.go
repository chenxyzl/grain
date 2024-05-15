package actor

import (
	"fmt"
	"strings"
)

func newActorRefWithKind(address string, kind string, name string) *ActorRef {
	return &ActorRef{
		Address: address,
		XPath:   fmt.Sprintf("kinds/%s/%s", kind, name),
	}
}

func newActorRefFromId(fullIdent string) *ActorRef {
	v := strings.Split(fullIdent, "@")
	if len(v) != 2 {
		return nil
	}
	return &ActorRef{Address: v[0], XPath: v[1]}
}

// GetId addres@ident
func (x *ActorRef) GetId() string {
	return fmt.Sprintf("%s@%s", x.GetAddress(), x.GetXPath())
}

// GetKind ...
func (x *ActorRef) GetKind() string {
	ident := x.GetXPath()
	if ident == "" {
		return ""
	}
	return strings.SplitN(ident, "/", 3)[1]
}

// GetName ...
func (x *ActorRef) GetName() string {
	ident := x.GetXPath()
	if ident == "" {
		return ""
	}
	return strings.SplitN(ident, "/", 3)[2]
}
