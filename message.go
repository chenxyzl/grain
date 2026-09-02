package grain

import (
	"github.com/chenxyzl/grain/message"
)

// Shared, preallocated framework messages: 1.0ns/0B to reuse one versus 36ns/64B to build it,
// and the framework needs them constantly.
//
// ⚠️ NONE of them may be mutated. *message.ErrCode is a generated struct with EXPORTED Code and
// Des fields, and both error values escape into user code — errActorNotFound comes back out of
// Ask, errAskNotRunning is returned by it. So one caller doing `err.Des = ctx + err.Des`
// permanently changes what every later Ask in the process observes, and races with concurrent
// readers. Go's own sentinels share safely only because their types are immutable (io.EOF's
// field is unexported); this one is not. Need context? Build your own from err.Code / err.Des.
var (
	// the `msg` prefix keeps `poison` from reading like iProcess.poison(), which stops a
	// process without going through the mailbox at all: control signal versus message.
	msgInitialize = &message.Initialize{}
	msgPoison     = &message.Poison{}

	// errActorNotFound is replied to an Ask whose target actor does not exist.
	errActorNotFound = message.WithErrCode(message.CodeActorNotFound, "actor not found")

	// errKindNotInCluster is replied to an Ask for a cluster kind no node hosts. Same code as
	// errActorNotFound — to the caller it is the same "target does not exist", so errors.Is on
	// CodeActorNotFound matches either — but a distinct Des: a missing WithConfigKind and a
	// merely-unactivated grain need different fixes.
	errKindNotInCluster = message.WithErrCode(message.CodeActorNotFound,
		"actor kind is not hosted by any node in the cluster")

	// errAskNotRunning is the reply for a blocking Ask outside the running phase.
	errAskNotRunning = message.WithErrCode(message.CodeAskNotRunning,
		"ask is only allowed while the actor is running: not from Started() (it cannot serve requests "+
			"yet, so the Ask may never be satisfiable) and not from PreStop() (blocking there re-enters "+
			"the stop path). Issue it from a normal handler — to Ask at startup, Tell self a message in "+
			"Started() and Ask when handling it")
)
