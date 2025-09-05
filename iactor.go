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
	_getRunningMsgId() uint64
	_setRunningMsgId(uint64)
	_cleanRunningMsgId()

	//Started after self Instance
	Started()
	//PreStop when receive poison, before stop self
	PreStop()
	//Receive message
	Receive(ctx Context)
}
