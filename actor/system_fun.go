package actor

import "context"

func buildContext() context.Context {
	ctx := context.WithValue(context.Background(), "foo", "bar")
	return ctx
}
