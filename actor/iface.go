package actor

type Producer func() IActor
type Receiver func(ctx IContext)
