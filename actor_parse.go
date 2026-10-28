package grain

import (
	"strings"
)

type cacheParse struct {
	d8c        string //direct or cluster
	kind       string
	name       string
	directAddr string
}

// parseCache splits "type/kind/name@directAddr" into those four parts. directAddr is "" when
// absent, and all four are "" if fullPath is not in that shape.
//
// Every result is a SUBSTRING of fullPath, so this allocates nothing — deliberately, because
// newActorRefFromAID runs it once for the target and once for the sender of EVERY inbound
// envelope. The two strings.Split calls it replaces cost 94ns and one allocation per call; this is
// 29ns and none.
func parseCache(fullPath string) (typ, kind, name, directAddr string) {
	i := strings.IndexByte(fullPath, '/')
	if i < 0 {
		return "", "", "", ""
	}
	rest := fullPath[i+1:]
	j := strings.IndexByte(rest, '/')
	if j < 0 {
		return "", "", "", ""
	}
	name = rest[j+1:]
	// a third separator means the input was not "a/b/c" — the same rejection as len(Split) != 3
	if strings.IndexByte(name, '/') >= 0 {
		return "", "", "", ""
	}
	typ, kind = fullPath[:i], rest[:j]
	if k := strings.IndexByte(name, '@'); k >= 0 {
		directAddr = name[k+1:]
		name = name[:k]
		// Matches Split("@")[1]: a second '@' ends directAddr. Unreachable for an id this package
		// builds — ValidateActorName bars '@' in a name, and addresses never contain one — but
		// fullPath also arrives off the wire, so keep the established shape for garbage rather
		// than inventing a new one here.
		if k2 := strings.IndexByte(directAddr, '@'); k2 >= 0 {
			directAddr = directAddr[:k2]
		}
	}
	return typ, kind, name, directAddr
}

// cacheParse is embedded BY VALUE in actorIdWrapper, so these are never called on a nil receiver —
// the getters have no nil guard for that reason.
func (x *cacheParse) GetType() string       { return x.d8c }
func (x *cacheParse) GetKind() string       { return x.kind }
func (x *cacheParse) GetName() string       { return x.name }
func (x *cacheParse) GetDirectAddr() string { return x.directAddr }
