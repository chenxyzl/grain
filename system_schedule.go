package grain

import (
	"time"

	"google.golang.org/protobuf/proto"
)

func (x *system) ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.timerSchedule.sendOnce(target, delay, msg)
}

func (x *system) ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.timerSchedule.sendRepeatedly(target, delay, interval, msg)
}
