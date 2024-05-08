package actor

type IActor interface {
	//inner api, for inherit auth
	_init(system *System, self *ActorRef, this IActor) //for bind self
	_getRunningMsgId() uint64
	_setRunningMsgId(uint64)
	_cleanRunningMsgId()

	//Started after self Instance
	Started() error
	//PreStop when receive poison, before stop self
	PreStop() error
	//Receive message
	Receive(ctx IContext)
}
