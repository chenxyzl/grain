package actor

type Receiver interface {
	Receive(ctx Context)
}

type messageInvoker interface {
	invoke(*MessageEnvelope)
}
