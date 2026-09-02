package grain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidName is returned by SpawnNamed, and carried by the panic GetClusterActorRef raises,
// when a name cannot appear in an actor id. Use errors.Is.
var ErrInvalidName = errors.New("grain: invalid actor name")

// An actor id is "type/kind/name" plus an optional "@addr", split on exactly those separators, so a
// name carrying one of them parses into a DIFFERENT id than it was built from — and, when the piece
// count no longer matches, into an id with every field empty. Such a ref is unroutable in a way
// nothing reports: isCluster() is false, so the address lookup returns "", the send is dropped, and
// no node can ever activate it. Grain names routinely come from user data, hence the check.
const invalidNameChars = "/@"

// ValidateActorName reports whether name may be used as an actor or grain name. Names must be
// non-empty and free of '/' and '@', which are the actor-id separators.
//
// Call it when the name comes from data you do not control — a player id, a room code, a tenant
// key. GetClusterActorRef panics on a bad name (it has no error to return), so pre-checking is how
// untrusted input gets turned into a business error instead of a crash:
//
//	if err := grain.ValidateActorName(id); err != nil {
//		return err
//	}
//	ref := system.GetClusterActorRef("player", id)
func ValidateActorName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidName)
	}
	if i := strings.IndexAny(name, invalidNameChars); i >= 0 {
		return fmt.Errorf("%w: %q contains %q, which separates the parts of an actor id",
			ErrInvalidName, name, name[i])
	}
	return nil
}

// validateKindName applies the same rule to a kind. Separate function so the message names the
// right thing; kinds come from WithConfigKind, i.e. from code, so this is a typo check.
func validateKindName(kind string) error {
	if kind == "" {
		return fmt.Errorf("%w: kind is empty", ErrInvalidName)
	}
	if i := strings.IndexAny(kind, invalidNameChars); i >= 0 {
		return fmt.Errorf("%w: kind %q contains %q, which separates the parts of an actor id",
			ErrInvalidName, kind, kind[i])
	}
	return nil
}
