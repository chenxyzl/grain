package actor

import "fmt"

// NewActorRef ...
func NewActorRef(address string, identifier string) *ActorRef {
	return &ActorRef{
		Address:    address,
		Identifier: identifier,
	}
}

// NewIdentifier ...
func NewIdentifier(kind, key string) *Identifier {
	return &Identifier{Kind: kind, Key: key}
}
func (x *ActorRef) GetId() string {
	return fmt.Sprintf("%s@%s", x.GetAddress(), x.GetIdentifier())
}
