package grain

import "github.com/chenxyzl/grain/message"

var initialize = &message.Initialize{}
var poison = &message.Poison{}
var errActorNotFound = &message.Error{Code: codeActorNotFound}
