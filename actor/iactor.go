package actor

// Producer actor producer
type Producer func() IActor

// IActor actor interface
type IActor interface {
	//inner api, for inherit auth
	_init(system *System, self *ActorRef, this IActor) //for bind self
	_getRunningMsgId() uint64
	_setRunningMsgId(uint64)
	_cleanRunningMsgId()
	//
	_preStart()
	_afterStop()

	//Started after self Instance
	Started()
	//PreStop when receive poison, before stop self
	PreStop()
	//Receive message
	Receive(ctx Context)
}
