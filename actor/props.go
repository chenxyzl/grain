package actor

type ProducerFunc func() IActor

type PropsOption func(props *Props)

type Props struct {
	kinds map[string]*ProducerFunc
}
