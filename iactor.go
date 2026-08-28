package grain

// iProducer actor producer
type iProducer func() IActor
type tKind struct {
	producer iProducer
	opts     []KindOptFunc
}

// IActor actor interface
type IActor interface {
	//inner api, for inherit auth
	_init(self ActorRef) //for bind self
	//_bindTurn binds the reentrancy turn controller of the owning processor, so
	//a blocking Ask can yield/reacquire the actor's single execution turn.
	_bindTurn(t reentryTurn)

	//Started after self Instance
	Started()
	//PreStop when receive poison, before stop self
	PreStop()
	//Receive message
	Receive(ctx Context)
}

// drainState is the per-drain-goroutine flag: once a goroutine yields its turn
// inside a blocking Ask (spawning a successor drainer), handedOff is set and the
// goroutine exits after finishing its current handler instead of looping.
type drainState struct {
	handedOff bool
}

// reentryTurn lets BaseActor.Ask (through askImpl) release the actor's execution
// turn before blocking on a reply and reacquire it afterwards, enabling
// reentrancy while keeping the actor strictly single-threaded. Implemented by
// processorMailBox.
type reentryTurn interface {
	// yieldTurn releases the turn before blocking (ensuring a successor drainer
	// exists) and returns the caller's drain state to restore on resume.
	yieldTurn() *drainState
	// resumeTurn reacquires the turn after the reply arrives.
	resumeTurn(ds *drainState)
}

