package grain

import (
	"strconv"

	"google.golang.org/protobuf/proto"
)

var _ ActorRef = (*replyRef)(nil)

// replyRef is a lightweight, non-registered ActorRef used as the sender of an
// Ask. It carries the correlation id (snId) and the origin node address so the
// reply can be routed back and delivered via the system's pending table. Unlike
// a real actor it is never registered; its string id is built lazily and only
// when it must be serialized for a cross-node send.
type replyRef struct {
	snId   uint64
	addr   string
	system ISystem
}

func newReplyRef(snId uint64, addr string, system ISystem) *replyRef {
	return &replyRef{snId: snId, addr: addr, system: system}
}

func (r *replyRef) GetSystem() ISystem { return r.system }
func (r *replyRef) GetKind() string    { return defaultReplyKind }
func (r *replyRef) GetName() string    { return strconv.FormatUint(r.snId, 10) }
func (r *replyRef) GetDirectAddr() string { return r.addr }

// GetId lazily builds the routable id; only needed when serialized for a remote
// send (stream_write) — local replies never call it.
func (r *replyRef) GetId() string {
	return defaultActDirect + "/" + defaultReplyKind + "/" + strconv.FormatUint(r.snId, 10) + "@" + r.addr
}

func (r *replyRef) isDirect() bool  { return true }
func (r *replyRef) isCluster() bool { return false }
func (r *replyRef) isAsk() bool     { return true }
func (r *replyRef) askSnId() uint64 { return r.snId }

func (r *replyRef) getRemoteAddrCache() string { return "" }

func (r *replyRef) Tell(msg proto.Message) {
	r.system.getSender().tell(r, msg)
}
