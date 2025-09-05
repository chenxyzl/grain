package grain

import (
	"strings"
)

type cacheRemote struct {
	version    int64
	remoteAddr string
}

type cacheParse struct {
	d8c        string //direct or cluster
	kind       string
	name       string
	directAddr string
}

// parseCache
// @return type
// @return kind
// @return name
// @return directAddr default is ""
func parseCache(fullPath string) (string, string, string, string) {
	a1 := strings.Split(fullPath, "/")
	if len(a1) != 3 {
		return "", "", "", ""
	}
	a2 := strings.Split(a1[2], "@")
	name := a2[0]
	var directAddr string
	if len(a2) > 1 {
		directAddr = a2[1]
	}
	return a1[0], a1[1], name, directAddr
}

func (x *cacheParse) GetType() string {
	if x == nil {
		return ""
	}
	return x.d8c
}
func (x *cacheParse) GetKind() string {
	if x == nil {
		return ""
	}
	return x.kind
}
func (x *cacheParse) GetName() string {
	if x == nil {
		return ""
	}
	return x.name
}
func (x *cacheParse) GetDirectAddr() string {
	if x == nil {
		return ""
	}
	return x.directAddr
}
