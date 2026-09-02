package grain

import (
	"time"

	"google.golang.org/protobuf/proto"
)

// ScheduleOnce delivers msg to target after delay. ⚠️ msg is delivered as-is — see IScheduler.
func (x *system) ScheduleOnce(target ActorRef, delay time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.timerSchedule.sendOnce(target, delay, msg)
}

// ScheduleRepeated delivers msg to target after delay, then every interval until the returned func
// is called. ⚠️ Every tick delivers the SAME instance — see IScheduler.
func (x *system) ScheduleRepeated(target ActorRef, delay time.Duration, interval time.Duration, msg proto.Message) CancelScheduleFunc {
	return x.timerSchedule.sendRepeatedly(target, delay, interval, msg)
}
