package uuid

var _uuid *UUID

func Init(nodeId uint64) error {
	uuid, err := NewUUID(nodeId)
	if err != nil {
		return err
	}
	_uuid = uuid
	return nil
}

// Generate gen global uuid
func Generate() uint64 {
	if _uuid == nil { //严重错误直接退出
		panic("uuid not init")
	}
	return _uuid.Generate()
}

// GetAskStartId return ask start id. node 占高 nodeBits 位, 低位全留给 nextSnId 自增, 各 node 区间不重叠。
func GetAskStartId() uint64 {
	if _uuid == nil { //严重错误直接退出
		panic("uuid not init")
	}
	return _uuid.node << (totalBits - nodeBits)
}

// ParseSortVal returns the id with its node bits removed.
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
