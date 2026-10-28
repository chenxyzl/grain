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
	_init(self ActorRef)               //for bind self
	_bindExecutionGate(*executionGate) // binds actor-local execution authority before publication

	//Started after self Instance
	Started()
	//PreStop when receive poison, before stop self
	PreStop()
	//Receive one message; the only place user logic runs
	Receive(ctx Context)
}
