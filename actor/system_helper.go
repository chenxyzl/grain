package actor

import "google.golang.org/protobuf/proto"

// NoReentrySend
// msg to target
// warning don't change msg value when send, may data race
func NoReentrySend(system *System, target *ActorRef, msg proto.Message) {
	system.sendWithoutSender(target, msg)
}

// NoReentryRequest sync request mean's not allowed re-entry
// wanted system.NoReentryRequest[T proto.Message](target *ActorRef, req proto.Message) T
// but golang not support
func NoReentryRequest[T proto.Message](system *System, target *ActorRef, req proto.Message) (T, error) {
	return request[T](system, target, req, system.getNextSnId())
}
