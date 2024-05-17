package actor

import (
	"google.golang.org/protobuf/proto"
	"time"
)

func (x *System) ScheduleOnce(target *ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.timerSchedule.sendOnce(target, delay, msg)
}

func (x *System) ScheduleRepeated(target *ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.timerSchedule.sendRepeatedly(target, delay, interval, msg)
}
