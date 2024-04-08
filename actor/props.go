package actor

type PropsOption func(props *Props)

type Props struct {
	kinds map[string]*tKind
}

type tKind struct {
	kind     string
	producer func(*System) IActor
}
