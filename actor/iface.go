package actor

import "google.golang.org/protobuf/proto"

type SenderWith func(target *ActorRef, msg proto.Message, sender *ActorRef, msgSnIds ...uint64)
type SenderWithOut func(target *ActorRef, msg proto.Message, msgSnIds ...uint64)
