package uuid

import (
	"fmt"
	"sync"
	"time"
)

/*
+-----------------------------------------------------------+
| 42 Bit Timestamp | 10 Bit NodeID | 12 Bit Sequence ID |
+-----------------------------------------------------------+
*/

const (
	totalBits uint8  = 64
	nodeBits  uint8  = 10                    // 节点 ID 的位数
	stepBits  uint8  = 12                    // 序列号的位数
	nodeMax   uint64 = -1 ^ (-1 << nodeBits) // 节点 ID 的最大值，用于检测溢出
	stepMax   uint64 = -1 ^ (-1 << stepBits) // 序列号的最大值，用于检测溢出
	timeShift uint8  = nodeBits + stepBits   // 时间戳向左的偏移量
	nodeShift uint8  = stepBits              // 节点 ID 向左的偏移量
	epoch     uint64 = 1672502400000         // timestamp 2023-01-01:00:00:00 GMT
)

type UUID struct {
	mu        sync.Mutex // 添加互斥锁，保证并发安全
	timestamp int64      // 时间戳部分
	node      uint64     // 节点 ID 部分
	step      uint64     // 序列号 ID 部分
}

// NewUUID 构造器
func NewUUID(node uint64) (*UUID, error) {
	// 如果超出节点的最大范围，产生一个 error
	if node > nodeMax {
		//return nil, errors.New("Node number must be between 0 and 1023")
		return nil, fmt.Errorf("node number must be between 0 and %d", nodeMax)
	}
	// 生成并返回节点实例的指针
	return &UUID{
		timestamp: 0,
		node:      node,
		step:      0,
	}, nil
}

// Generate 生成唯一id
func (n *UUID) Generate() uint64 {
	n.mu.Lock() // 保证并发安全, 加锁

	// 获取当前时间的时间戳 (毫秒数显示)
	now := time.Now().UnixMilli()

	if n.timestamp == now {
		n.step = (n.step + 1) & stepMax

		// 当前 step 用完
		if n.step == 0 {
			// 等待本毫秒结束
			for now <= n.timestamp {
				now = time.Now().UnixMilli()
			}
		}

	} else {
		// 本毫秒内 step 用完
		n.step = 0
	}

	n.timestamp = now
	// 移位运算，生产最终 ID
	result := (uint64(now)-epoch)<<timeShift | (n.node << nodeShift) | (n.step)

	n.mu.Unlock() // 方法运行完毕后解锁

	return result
}

func MaxNodeMax() uint64 {
	return nodeMax
}
