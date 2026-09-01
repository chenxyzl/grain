package grain

import (
	"github.com/chenxyzl/grain/message"
)

// Shared, preallocated framework messages.
//
// All of these are handed out repeatedly rather than rebuilt per use: at 1.0ns/0B
// versus 36ns/64B for constructing one, and given how often the framework needs them,
// preallocating is the right call.
//
// ⚠️ NONE of them may be mutated. That is not merely style: msgInitialize and
// msgPoison are fieldless so there is nothing to write, but *message.ErrCode is a
// generated struct with EXPORTED Code and Des fields, and both error values escape all
// the way into user code — errActorNotFound is Tell'd to the asking actor and comes
// back out of Ask, errAskNotRunning is returned by Ask directly. So a caller doing the
// natural thing:
//
//	reply, err := x.Ask[*pb.Reply](ref, req)
//	if err != nil {
//	    err.Des = "player " + id + ": " + err.Des // ← corrupts it process-wide
//	}
//
// permanently changes what every later Ask in the process observes, and races with any
// concurrent reader. (Go's own sentinel errors get away with sharing because their
// types are immutable — io.EOF is an *errors.errorString whose field is unexported.
// That does not hold here.)
//
// If you need to add context, build your own ErrCode from err.Code / err.Des.
var (
	// msgInitialize / msgPoison carry the `msg` prefix on purpose: `poison` alone read
	// exactly like iProcess.poison(), the method that stops a process without going
	// through the mailbox at all. One is a control signal, the other a message — at the
	// call site `v.poison()` and `x.tell(ref, poison)` are two different mechanisms, and
	// sharing a name hid that.
	msgInitialize = &message.Initialize{}
	msgPoison     = &message.Poison{}

	// errActorNotFound is replied to an Ask whose target actor does not exist.
	errActorNotFound = message.WithErrCode(message.CodeActorNotFound, "actor not found")

	// errKindNotInCluster is replied to an Ask whose target is a cluster kind that no
	// node in the cluster hosts. Same code as errActorNotFound — from the caller's side
	// it is the same "your target does not exist" answer, so errors.Is against
	// message.CodeActorNotFound matches either — but a distinct Des, because the causes
	// need different fixes: a missing WithConfigKind, versus a grain that is simply not
	// activated.
	errKindNotInCluster = message.WithErrCode(message.CodeActorNotFound,
		"actor kind is not hosted by any node in the cluster")

	// errAskNotRunning is the reply for a blocking Ask attempted outside the actor's
	// running phase (from Started() or PreStop()).
	errAskNotRunning = message.WithErrCode(message.CodeAskNotRunning,
		"ask is only allowed while the actor is running: not from Started() (it cannot serve requests "+
			"yet, so the Ask may never be satisfiable) and not from PreStop() (blocking there re-enters "+
			"the stop path). Issue it from a normal handler — to Ask at startup, Tell self a message in "+
			"Started() and Ask when handling it")
)
