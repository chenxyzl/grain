package actor

import "log/slog"

type ReceiverFunc func(c IContext)

type Behavior []ReceiverFunc

func NewBehavior() Behavior {
	return make(Behavior, 0)
}

func (b *Behavior) Become(receive ReceiverFunc) {
	b.clear()
	b.push(receive)
}

func (b *Behavior) BecomeStacked(receive ReceiverFunc) {
	b.push(receive)
}

func (b *Behavior) UnbecomeStacked() {
	b.pop()
}

func (b *Behavior) Receive(context IContext) {
	behavior, ok := b.peek()
	if ok {
		behavior(context)
	} else {
		context.Logger().Error("empty behavior called", slog.Any("actor", context.Self()))
	}
}

func (b *Behavior) clear() {
	if len(*b) == 0 {
		return
	}

	for i := range *b {
		(*b)[i] = nil
	}

	*b = (*b)[:0]
}

func (b *Behavior) peek() (v ReceiverFunc, ok bool) {
	if l := b.len(); l > 0 {
		ok = true
		v = (*b)[l-1]
	}

	return
}

func (b *Behavior) push(v ReceiverFunc) {
	*b = append(*b, v)
}

func (b *Behavior) pop() (v ReceiverFunc, ok bool) {
	if l := b.len(); l > 0 {
		l--

		ok = true
		v = (*b)[l]
		(*b)[l] = nil
		*b = (*b)[:l]
	}

	return
}

func (b *Behavior) len() int {
	return len(*b)
}
