package grain

// Producer builds a new actor instance; the framework calls it once per activation.
type Producer func() IActor
type tKind struct {
	producer Producer
	opts     []KindOptFunc
}

// IActor actor interface
type IActor interface {
	//inner api, for inherit auth
	_init(self ActorRef) //for bind self
	//_bindTurn binds the owning processor's turn controller, so a blocking Ask can yield it
	_bindTurn(t reentryTurn)

	//Started after self Instance
	Started()
	//PreStop when receive poison, before stop self
	PreStop()
	//Receive one message; the only place user logic runs
	Receive(ctx Context)
}

// drainState flags a drain goroutine that yielded its turn inside a blocking Ask (thereby
// spawning a successor drainer): it exits after the current handler instead of looping.
type drainState struct {
	handedOff bool
}

// reentryTurn lets askImpl release the actor's execution turn before blocking on a reply and
// reacquire it after — reentrancy while staying single-threaded. Implemented by processorMailBox.
type reentryTurn interface {
	// yieldTurn releases the turn (ensuring a successor drainer exists) and returns the state to
	// restore on resume.
	yieldTurn() *drainState
	// resumeTurn reacquires the turn after the reply arrives.
	resumeTurn(ds *drainState)
	// isStarted: actor is running (Started() done, PreStop() not begun) — the ONLY phase allowing
	// a blocking Ask, so askImpl allow-lists it. Valid only while holding the turn, as handlers do.
	isStarted() bool
}
