package actor

type Receiver func(ctx IContext)

type messageInvoker interface {
	invoke(ctx IContext)
}
