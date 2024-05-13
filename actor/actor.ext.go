package actor

import (
	"fmt"
	"strings"
)

// NewActorRefLocal ...
// todo replace with system.GetRef[T IActor](id string/int) -> system.GetRef(kind string,id string/int)
func NewActorRefLocal(address string, identifier string) *ActorRef {
	return &ActorRef{
		Address:    address,
		Identifier: fmt.Sprintf("kinds/%s/%s", defaultKindName, identifier),
	}
}

func NewActorRefWithKind(address string, kind string, id string) *ActorRef {
	return &ActorRef{
		Address:    address,
		Identifier: fmt.Sprintf("kinds/%s/%s", kind, id),
	}
}

// NewIdentifier ...
func NewIdentifier(kind, key string) *Identifier {
	return &Identifier{Kind: kind, Key: key}
}

// GetId addres@ident
func (x *ActorRef) GetId() string {
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

// GetKind ...
func (x *ActorRef) GetName() string {
	ident := x.GetIdentifier()
	if ident == "" {
		return ""
	}
	return strings.SplitN(ident, "/", 3)[2]
}
