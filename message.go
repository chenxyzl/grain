package grain

import (
	"github.com/chenxyzl/grain/message"
)

var initialize = &message.Initialize{}
var poison = &message.Poison{}
var errActorNotFound = message.WithErrCode(message.CodeActorNotFound, "actor not found") //errors.New("actor not found")

// errAskNotRunning is the preset reply for a blocking Ask attempted outside the
// actor's running phase. It is a shared immutable singleton, like errActorNotFound.
var errAskNotRunning = message.WithErrCode(message.CodeAskNotRunning,
	"ask is only allowed while the actor is running: not from Started() (it cannot serve requests "+
		"yet, so the Ask may never be satisfiable) and not from PreStop() (blocking there re-enters "+
		"the stop path). Issue it from a normal handler — to Ask at startup, Tell self a message in "+
		"Started() and Ask when handling it")
