# 剩余待办（供你放进新 session）

## 已修（本轮）

  1 + N3. 一起修了，因为是同一次 Get 的两个问题。
     原来是 register()（每个候选 id 一次 Txn）→ watch()（自己再 Get 一次），
     所以 (a) 第 N 个节点要 N 次往返，200 节点 = 2 万次事务；
     (b) 从发布自己的 member key 到 watch 的 Get 落地这段时间 nodeMap 是空的，
     而 grpc 从 system.Start() 第一步就在 accept 了 → 这期间进来的 cluster envelope 被丢弃。
     现在：start() 里先 loadMembers() 做**唯一一次** Get（填 nodeMap + 告诉 register 哪些 id 空闲
     + 返回 watch 要 anchor 的 revision）→ register() 一次 Txn → watchMembers(rev)。
     候选 id **随机**取（lowest-free 在同时启动时仍退化成 O(N) 轮）。
     抢到后立刻把自己写进 nodeMap，否则 CalcAddrByKind8Name 看不到自己、会把自己的 grain 路由给别人。
     顺带删掉了 parseMemberId —— nodeMap 的 key 本来就是 id。
     测试：TestFreeNodeIds / TestClaimableNodeIdsAreAcceptedByUuid

  8. askId 与 addr/logger 伪共享同一 cache line → 前后各加一段 padding。
     16 核实测 24.1 → 14.2 ns/op。TestSystemHotFieldsAreOnSeparateCacheLines 断言的是
     "不在同一 cache line" 这个性质而非固定偏移 —— 以后在 askId 上方加字段会立刻红。

  14b. grain 注册锁被抢锁失败方误删 → 同一 grain 真的双活。两个独立缺陷各修一处：
     value 从 ref.GetDirectAddr()（cluster ref 恒为 ""）改成 config.state.Address；
     加 registered 标记，只在本次注册确实成功时才 unregister。
     3 个回归测试均已验证在旧代码上失败。

  N1. streamWriteActor 的 Recv switch：status.Code(nil)==codes.OK，所以 err==nil（对端在只写流上
     发消息，协议违规）被误判成"正常关闭"，而专门为它写的分支不可达；`case err != nil` 同样死。
     外层 for 也是死循环（SA4004）。所有分支最后都走 Poison(self)，所以后果纯粹是日志误导 ——
     修的就是分支顺序（err==nil 提到最前）。和 remote/stream_server.go Listen 里已修过的同类。

  N2. Ask 发给"没有任何节点承载的 cluster kind"时不回错误 → 干等一个 askTimeout。
     现在回 errKindNotInCluster（同 CodeActorNotFound，errors.Is 一次覆盖；Des 单独区分，
     因为修法不同：漏了 WithConfigKind ≠ grain 没激活）。

  N4. system.logger 从普通字段改成 atomic.Pointer。Start()/init() 写它时 grpc 已在 accept，
     RecvEnvelope 的 Logger() 读会和它并发。旧写法在 -race 下报 WARNING: DATA RACE
     （TestSystemLoggerIsRaceFree 复现）。

  13. WithConfigLogger + NewLogger（详见 CHANGELOG）。

  6. addr_hash 加了 murmur3 的 fmix32 终结器。rendezvous 取 argmax，所以需要高位混得开，
     而 FNV-1a 最后一步只有 `h ^= c; h *= prime`，高位雪崩弱、且对共享前缀的输入相关 ——
     这里所有候选共享同一个 name、只有后面的地址不同，正是最坏情况。
     1M key 偏离均值：10 节点 ±11/16% → ±0.3/0.4%；50 节点 → ±1.3/2.0%；200 节点 → ±3.9%。
     代价约 3%（BenchmarkCalcAddr 537 → 551 ns/op，仍 0 alloc）。
     ⚠️ 改了 key→owner 映射，不能灰度 —— 两种版本混跑会对每个 key 的 owner 都不一致。趁上线前做掉。
     TestOwnershipIsEvenlySpread 钉住分布；节点数故意取小，因为每节点 key 数太少时采样噪声会盖掉信号
     （200 节点 / 20 万 key 每节点只有 1000 个，±3% 纯属噪声）。
     TestFmix32MatchesMurmur3 钉常数和位移（对照公开向量 MurmurHash3_x86_32("",seed=1)=0x514E28B7）——
     常数打错的话输出照样"看起来随机"、分布照样均匀，分布测试根本抓不到，只会静默地把 key 全换个 owner。
     顺带 review 出两处（同属 addr_hash 内部）：
     (a) name 的 hash 原来每个候选都重算一遍。FNV-1a 是流式的，所以
         fnv32aAdd(fnv32aStart(name), addr) 与 fnv32aTwo(name, addr) **位级相同**，
         提到循环外：551 → 325 ns/op（200 节点 5784 → 3232），owner 映射不变。
         位级相同这点本来就被 TestCalcAddrMatchesReferenceImplementation 盯着（对照 hash/fnv 的朴素实现，
         8 种节点集形态 × 2000 个 name）。
     (b) Address 为空的成员不再参与选举。parseWatch 只丢 JSON 解析失败的值，不丢解析出空 Address 的，
         这种条目原来会赢下 ~1/N 的 key 并被当成 owner 返回，而调用方把 "" 读成"这个 kind 没人承载"
         → 静默丢消息。跳过它也让 maxAddr=="" 只剩一个含义（还没选过），score 为 0 才能正常胜出。

  14 的保护. 「不会乒乓」这个不变量原来没有任何测试/注释守着，现在由
     TestForwardingCannotLoop（2 万次随机成员视图穷举）+ TestScoreIsIndependentOfTheMemberView 守着。
     注意测试守的是**选择结果**而非原始 score：注入的视图依赖只有大到能改变 argmax 才会成环，
     测试注释里记了三种注入分别是什么结果。

## 已决定不做

  2. 关停顺序（你说跳过）。核对补充：暴露窗口不是 stopWaitTimeSecond 而是
     **2×(N-1) = 默认 4 秒** —— stopActorsImpl 首轮不 sleep、之后每轮 sleep 1s、times>=N 才 break
     ⇒ 单次 (N-1) 秒，而它被调用两次(first/latter)。另：draining 只拦
     ensureClusterKindActorExist 的按需激活，不拦发给已存在 actor 的消息，所以这 4 秒里消息
     照常投递进正在停止的 mailbox → 丢弃/dead letter。

  7. Envelope 池化 —— **grpc 明确禁止,不能做**。grpc@v1.82.1/stream.go 里
     ClientStream.SendMsg 的契约原文:"It is not safe to modify the message after calling
     SendMsg. Tracing libraries and stats handlers may use the message lazily."
     复用 Envelope 就是在下一条消息里改它;MarshalAppend 复用 buffer 也一样,因为
     Envelope.Content 指向那个 buffer。只要可能有 stats handler 还在读上一条消息就不安全 ——
     而 WithConfigGrpcDialOptions 允许用户装任意 handler,框架无法保证"我们不用 stats handler"。
     alloc 数据留档（供以后 grpc 契约变化时重新评估）:
       remote Tell (sender nil)      166 ns/op  2 allocs  168 B —— Marshal content 24B + Envelope 结构体 144B
       remote Ask  (replyRef sender) 263 ns/op  4 allocs  232 B —— 多出 replyRef.GetId() 的 FormatUint + 五段拼接
     string(proto.MessageName(msg)) 是 0 alloc（FullName 本身就是 string）。

  9. 保持 slog，不换库。代码侧已确认没有"过滤前就求值参数"的调用点拖累它
     （5 处 ghelper.StackTrace() 全在 Error 级 panic 恢复路径上，Error 基本不会被过滤）。

  11. al/safemap 是公开 API，那 ~150 行未被本仓库调用的导出方法保留。
     （现在是 420 行，死代码约 145-160 行 ≈ 35-38%）

## 剩余待办

  性能 B 组（都动并发核心，建议每项单独一轮 + 压测 + race）
  3. Ask 每消息新建 goroutine（−300~400ns / −2 allocs）—— 需先给 ringbuffer 加无锁 size 原子计数器。
     核对补充：**有两处 spawn**，不是一处 ——
       processor_mailbox.go schedule() 的 `go x.process()`：每条消息只要 actor 刚 idle 就新建，
         请求/应答型负载里就是"每消息一个"；
       yieldTurn() 的 `go x.process()`：每次阻塞 Ask 一个后继 drainer。
     前置条件成立：rb.Len() 确实要拿 mutex（ringbuffer.go:120-123），而 idle/running 的重新武装
     （process():268）和 doStop():311 都靠它。11.3% CPU 这个数字我没复现。

  4. Ask future 池化。试做过一版（7→5 allocs / 353→225 B / 1357→1259 ns/op），本次回退了，
     但结论留下：
     (a) cancelAsk 现在调的是 Remove（system.go），**没有返回值、压根无法知道是不是自己赢的**，
         池化前必须先改成 Pop —— 这一点原文说对了；
     (b) **但光有这条不够**：sender 有两个，deliverReply 会 Pop，而 wakePendingAsks 只发不 Pop。
         不变量必须是「所有 sender 发送前都先 Pop」才成立，所以 wakePendingAsks 也得改成
         collect-then-Pop（不能在 IterCb 里 Pop：IterCb 持分片读锁而 Pop 要同一分片写锁 → 死锁）；
     (c) 有了独占性，回收点只有两处：awaitReply **收到值**时（常见路径，池化的收益全在这里）、
         cancelAsk **自己 Pop 成功**时。reply 与 timeout 打平时两者都不成立，channel 交给 GC。
     写错的形态很危险：一个 Ask 会拿到别人的回复。做的时候必须配一个"每个 Ask 校验拿到的是
     自己的回声"的并发测试 —— 我验证过，对"timeout 分支也回收"这种写法它会立刻报出
     `Ask for "w7/4" got a reply for "w6/4"`。

  5. turn 改为每批 drain 收放一次（−35ns/消息）。turn 确实每消息 acquire/release
     （processor_mailbox.go 的 run 循环）。漏掉的风险：doStop 也拿 turn，
     yieldTurn/resumeTurn 的 handoff 协议假定"每消息持有"，批量化要连协议一起改，不只是挪两行。

  设计 / 运维
  14. Envelope 仍然没有 hop/TTL 字段。目前靠上面那个单调性不变量兜底，已加测试。
     如果哪天真要加节点权重 / 最少负载路由，先加 hop 上限。
