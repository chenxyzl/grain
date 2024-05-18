package actor

import "google.golang.org/protobuf/proto"

type Sender func(target *ActorRef, msg proto.Message, sender *ActorRef, msgSnIds ...uint64)
