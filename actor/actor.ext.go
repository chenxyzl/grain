package actor

import (
	"fmt"
	"strings"
)

func newActorRefWithKind(address string, kind string, name string) *ActorRef {
	return &ActorRef{
		Address:    address,
		Identifier: fmt.Sprintf("kinds/%s/%s", kind, name),
	}
}

// GetFullIdentifier addres@ident
func (x *ActorRef) GetFullIdentifier() string {
	return fmt.Sprintf("%s@%s", x.GetAddress(), x.GetIdentifier())
}

// GetKind ...
func (x *ActorRef) GetKind() string {
	ident := x.GetIdentifier()
	if ident == "" {
		return ""
	}
	return strings.SplitN(ident, "/", 3)[1]
}

// GetName ...
func (x *ActorRef) GetName() string {
	ident := x.GetIdentifier()
	if ident == "" {
		return ""
	}
	return strings.SplitN(ident, "/", 3)[2]
}
