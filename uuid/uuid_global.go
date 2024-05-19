package uuid

import "time"

var guuid *UUID

func Init(nodeId uint64) error {
	//全局的uuid生成器
	uuid, err := NewUUID(nodeId)
	if err != nil {
		return err
	}
	guuid = uuid
	return nil
}

// Generate gen global uuid
func Generate() uint64 {
	if guuid == nil { //严重错误直接退出
		panic("uuid not init")
	}
	return guuid.Generate()
}

// GetBeginRequestId return request begin id
func GetBeginRequestId() uint64 {
	if guuid == nil { //严重错误直接退出
		panic("uuid not init")
	}
	return guuid.node << (totalBits - nodeBits) & uint64(time.Now().UnixNano())
}

// ParseSortVal
// @return remove node
func ParseSortVal(id uint64) uint64 {
	return ((id >> timeShift) << timeShift) | ((id << (totalBits - stepBits)) >> (totalBits - stepBits))
}
func ParseTime(id uint64) uint64 {
	return id >> timeShift
}
func ParseNode(id uint64) uint64 {
	return (id << (totalBits - timeShift)) >> (totalBits - timeShift + stepBits)
}
func ParseStep(id uint64) uint64 {
	return (id << (totalBits - stepBits)) >> (totalBits - stepBits)
}