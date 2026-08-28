package grain

import (
	"github.com/chenxyzl/grain/message"
)

// initialize and poison are sentinel messages with no fields, so sharing one
// instance is safe — there is nothing in them to mutate.
var initialize = &message.Initialize{}
var poison = &message.Poison{}

// The error replies below are built fresh per use rather than shared. *ErrCode is a
// generated struct with exported, mutable fields, and these values escape all the
// way into user code — errActorNotFound() is Tell'd to the asking actor and comes
// back out of Ask, errAskNotRunning() is returned from Ask directly. A single caller
// doing `err.Des += ctx` on a shared instance would corrupt it for every later Ask in
// the process. Both sit on cold paths, so the allocation is irrelevant.

// errActorNotFound is replied to an Ask whose target actor does not exist.
func errActorNotFound() *message.ErrCode {
	return message.WithErrCode(message.CodeActorNotFound, "actor not found")
}

// errAskNotRunning is the reply for a blocking Ask attempted outside the actor's
// running phase (from Started() or PreStop()).
func errAskNotRunning() *message.ErrCode {
	return message.WithErrCode(message.CodeAskNotRunning,
		"ask is only allowed while the actor is running: not from Started() (it cannot serve requests "+
			"yet, so the Ask may never be satisfiable) and not from PreStop() (blocking there re-enters "+
			"the stop path). Issue it from a normal handler — to Ask at startup, Tell self a message in "+
			"Started() and Ask when handling it")
}
