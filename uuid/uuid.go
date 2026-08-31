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
	// 1735689600000 = 2025-01-01T00:00:00Z (真 UTC)。(42 位毫秒 = 139.4 年, 用尽点从 2162-05 推到 2164-05)。
	epoch uint64 = 1735689600000
)

type UUID struct {
	mu        sync.Mutex // 添加互斥锁，保证并发安全
	timestamp int64      // 时间戳部分
	node      uint64     // 节点 ID 部分
	step      uint64     // 序列号 ID 部分

	// 时钟基准: 构造时采样墙上时间一次, 之后一律用**单调时钟**推进 —— 见 nowMs。
	// 二者来自同一次 time.Now() 采样, 所以没有间隙。
	baseWall int64     // 构造时刻的 Unix 毫秒(已钳到 epoch)
	baseMono time.Time // 同一次采样, 携带 monotonic reading
}

// NewUUID 构造器
func NewUUID(node uint64) (*UUID, error) {
	// 如果超出节点的最大范围，产生一个 error
	if node > nodeMax {
		//return nil, errors.New("Node number must be between 0 and 1023")
		return nil, fmt.Errorf("node number must be between 0 and %d", nodeMax)
	}
	base := time.Now()
	// epoch 下限保护: id 布局编码的是 (ms - epoch), 且该减法是 uint64 运算。若构造
	// 时系统时钟早于 2023-01-01 (容器时钟未同步 / RTC 掉电), 减法会下溢成一个巨大
	// 的值 —— 实测 2020 年的时钟产出 18049687768967155712, 而正常 id 是
	// 485046263180431360, 大 37 倍: 既破坏 ParseSortVal 的排序, 又会和真实节点约
	// 133 年后产出的 id 相撞。钳到 epoch 后 id 仍然合法且单调, 好过越界。
	//
	// 只需在这里钳一次: nowMs 从这个基准出发单调递增, 不会再降到 epoch 以下。
	baseWall := max(base.UnixMilli(), int64(epoch))
	// 生成并返回节点实例的指针
	return &UUID{
		timestamp: 0,
		node:      node,
		step:      0,
		baseWall:  baseWall,
		baseMono:  base,
	}, nil
}

// nowMs 返回当前的 Unix 毫秒, 但走**单调时钟**: 构造时采样的墙上时间, 加上此后
// 由 monotonic clock 度量的经过时间。
//
// 两个好处:
//
//   - 结构性免疫时钟回拨。monotonic clock 不受 NTP 步进、手工改表、RTC 跳变影响,
//     所以返回值永不回退 —— 从源头消掉了"时钟往回走导致重复 id"这一整类问题,
//     而不是事后补救。
//
//   - time.Now() 底层的 now() 返回 (sec, nsec, mono) —— 墙上时钟和单调时钟各读一次;
//     而 runtimeNano() 只读单调时钟。省下的就是那一次墙上时钟读取。
//
// 代价: monotonic clock 不跟随 NTP 的缓步校准(slew), 所以长时间运行后本函数返回的
// 时间会与真实墙上时间产生漂移(量级取决于本机时钟漂移率, 典型每天毫秒级)。id 只
// 要求唯一且大致时序, 所以无影响; 但 ParseTime 解出的时间会带上这份漂移。进程重启
// 会重新采样基准, 漂移随之归零。
func (n *UUID) nowMs() int64 {
	return n.baseWall + time.Since(n.baseMono).Milliseconds()
}

// Generate 生成唯一id
func (n *UUID) Generate() uint64 {
	n.mu.Lock() // 保证并发安全, 加锁

	// 单调时钟推进的当前毫秒, 见 nowMs。已保证 >= epoch 且不会回退。
	now := n.nowMs()

	// 这里仍需要下界钳制, 但**不再是为了防时钟回拨**(nowMs 已经不会回退了), 而是
	// 为了下面"借用下一毫秒"的场景: step 用尽时 n.timestamp 会被推到墙上时钟之前,
	// 之后若干次调用的 now 都会小于它, 必须沿用 n.timestamp 继续递增 step。
	if now < n.timestamp {
		now = n.timestamp
	}

	if n.timestamp == now {
		n.step = (n.step + 1) & stepMax

		// 当前 step 用完: 等下一毫秒
		if n.step == 0 {
			// 等待本毫秒结束
			for now <= n.timestamp {
				now = n.nowMs()
			}
		}

	} else {
		// 进入了新的毫秒, 序列号归零
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
