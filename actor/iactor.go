package actor

type IActor interface {
	//inner api, for inherit auth
	init(system *System, self *ActorRef, this IActor) //for bind self
	//logger

	//Started after self Instance
	Started() error
	//PreStop when receive poison, before stop self
	PreStop() error
	//Receive message
	Receive(ctx IContext)
}
