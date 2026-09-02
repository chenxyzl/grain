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
	timeShift        = nodeBits + stepBits   // 时间戳向左的偏移量
	nodeShift        = stepBits              // 节点 ID 向左的偏移量

	// epoch 是 id 时间戳字段的起点: id 编码的是 (当前毫秒 - epoch)。
	// 1735689600000 = 2025-01-01T00:00:00Z (真 UTC); 42 位毫秒 = 139.4 年, 2164-05 用尽。
	epoch uint64 = 1735689600000
)

type UUID struct {
	mu        sync.Mutex // 添加互斥锁，保证并发安全
	timestamp int64      // 时间戳部分
	node      uint64     // 节点 ID 部分
	step      uint64     // 序列号 ID 部分

	// 时钟基准: 构造时采样墙上时间一次, 之后一律用单调时钟推进(见 nowMs); 二者同一次采样。
	baseWall int64     // 构造时刻的 Unix 毫秒(已钳到 epoch)
	baseMono time.Time // 同一次采样, 携带 monotonic reading
}

// NewUUID 构造器
func NewUUID(node uint64) (*UUID, error) {
	if node > nodeMax {
		return nil, fmt.Errorf("node number must be between 0 and %d", nodeMax)
	}
	base := time.Now()
	// epoch 下限保护: (ms - epoch) 是 uint64 减法, 构造时系统时钟早于 epoch(容器时钟未同步 /
	// RTC 掉电)会下溢成巨值, 破坏 ParseSortVal 排序且与约 133 年后的真 id 相撞。只需钳这一次:
	// nowMs 从此基准单调递增, 不会再降到 epoch 以下。
	baseWall := max(base.UnixMilli(), int64(epoch))
	return &UUID{
		timestamp: 0,
		node:      node,
		step:      0,
		baseWall:  baseWall,
		baseMono:  base,
	}, nil
}

// nowMs 返回当前的 Unix 毫秒, 走单调时钟: 构造时采样的墙上时间 + 此后 monotonic clock 度量
// 的经过时间。因此结构性免疫时钟回拨(NTP 步进、手工改表、RTC 跳变都影响不到它), 永不回退。
// 代价: 不跟随 NTP 缓步校准(slew), 长跑后与真实墙上时间漂移(典型每天毫秒级) —— id 只需唯一
// 且大致时序故无影响, 但 ParseTime 解出的时间会带上漂移, 进程重启重新采样即归零。这与 id 的
// 时间戳字段无关: Generate 在 step 用尽时会等待, 故 n.timestamp 永不超过本函数的返回值。
func (n *UUID) nowMs() int64 {
	return n.baseWall + time.Since(n.baseMono).Milliseconds()
}

// Generate 生成唯一id
func (n *UUID) Generate() uint64 {
	n.mu.Lock()

	// 单调时钟推进的当前毫秒, 见 nowMs。已保证 >= epoch 且不会回退。
	now := n.nowMs()

	// 下界钳制。当前设计下正常永不触发(nowMs 单调不减, 而 n.timestamp 只会被赋为过去某次
	// nowMs 的返回值), 纯防御: 万一 baseMono 的 monotonic reading 被剥掉(有人加了 .UTC()/
	// .Round(), 见 TestBaseMonoCarriesMonotonicReading), 这里是阻止发出重复 id 的最后一道闸。
	if now < n.timestamp {
		now = n.timestamp
	}

	if n.timestamp == now {
		n.step = (n.step + 1) & stepMax

		// 当前毫秒的 step 用完: 等到下一毫秒边界, 不"借用下一毫秒"。借用快约 7.5 倍, 但会让逻辑
		// 时间戳跑到真实时钟之前, 而进程重启重新以墙上时钟为基准, 于是从已用过的时间戳继续发号
		// 并大量碰撞。等待保证 timestamp <= 真实时钟; 因 nowMs 无回拨, 该等待有界(最多 1ms)。
		if n.step == 0 {
			for now <= n.timestamp {
				now = n.nowMs()
			}
		}

	} else {
		// 进入了新的毫秒, 序列号归零
		n.step = 0
	}

	n.timestamp = now
	result := (uint64(now)-epoch)<<timeShift | (n.node << nodeShift) | (n.step)

	n.mu.Unlock()

	return result
}

func MaxNodeMax() uint64 {
	return nodeMax
}
