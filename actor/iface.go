package actor

type Receiver interface {
	Receive(ctx IContext)
}

type messageInvoker interface {
	invoke(*MessageEnvelope)
}
