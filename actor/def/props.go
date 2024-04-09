package def

import "github.com/chenxyzl/grain/actor"

type PropsOption func(props *Props)

type Props struct {
	kinds map[string]*tKind
}

type tKind struct {
	kind     string
	producer func(*actor.System) actor.IActor
}
