package actor

import (
	"log/slog"
)

var _ IActor = (*processorActor)(nil)

type processorActor struct {
	IActor
	self   *ActorRef
	logger *slog.Logger
}

func newProcessorActor(self *ActorRef) IActor {
	ret := &processorActor{}
	ret.bind(ret, self)
	return ret
}

func (p *processorActor) bind(system *System, this IActor, self *ActorRef) {
	p.IActor = this
	p.self = self
	p.logger = slog.With("processorActor", self)
}

func (p *processorActor) Self() *ActorRef {
	return p.self
}

func (p *processorActor) Logger() *slog.Logger {
	return p.logger
}

func (p *processorActor) Awake() error {
	return nil
}

func (p *processorActor) Start() error {
	return nil
}

func (p *processorActor) Stop() error {
	return nil
}

func (p *processorActor) receive(ctx IContext) {
}

func (p *processorActor) Receive(ctx IContext) {
}
