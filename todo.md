# 剩余待办（供你放进新 session）

  已确认的 bug，未修
  1. provider_etcd.go node-id 线性扫描：第 N 个加入的节点要 N 次 etcd 往返，200 节点重启约 2 万次事务
  2. 关停顺序：集群注册在排空 actor 之后才摘除，对端在 stopWaitTimeSecond 内仍会路由过来（你之前说跳过）

  性能，B 组（都动并发核心，建议每项单独一轮 + 压测 + race）
  3. Ask 每消息新建 goroutine（newproc 占 11.3% CPU，−300~400ns / −2 allocs）—— 需先给 ringbuffer 加无锁 size 原子计数器
  4. Ask future 池化（−3 allocs / −176B / −150ns）—— 回收条件必须是「cancelAsk 自己 Pop 成功」，否则 use-after-recycle
  5. turn 改为每批 drain 收放一次（−35ns/消息）
  6. addr_hash 负载不均 19–28%（加 murmur3 fmix32 终结器可降到 <1%，但会改变 key→owner 映射，需全集群协调重启）
  7. 远端发送每消息 3 allocs（复用 Envelope + MarshalAppend）
  8. askId 与 addr 伪共享同一 cache line（2 行改动）

  日志库（我测完了但没来得及汇报）
  9. 每条记录：zerolog 118ns / phuslu 179ns / zap 460ns / slog 704ns；With 强制逃逸后 zerolog 205ns+1alloc vs slog 351ns+6allocs。但 slog 是标准库、Logger() 已是懒构建、且被过滤的日志只要 10ns。换库要动公开
  API（Logger() *slog.Logger）—— 值不值得取决于你的日志量级

  设计 / 运维
  11. concurrentmap.go 约 200/396 行死代码
  14. 【已验证：乒乓不会发生】score = fnv32a(name, addr) 只依赖 (name,地址)、与节点视图无关，
      且每个节点必然能看到自己（register 的 put 先于 watch 的 Get），所以每一跳在 (score,-addr)
      上严格单调 → 不可能成环；20 万次随机成员视图穷举 0 环，最长 5 跳（12 节点）。
      Envelope 里确实没有 hop/TTL 字段，但这个不变量目前没有任何测试或注释保护 ——
      一旦 hash 改成带权重或依赖视图，就没有兜底了。
  14b.【新，高优先级 bug】grain 注册锁会被抢锁失败的一方删掉，导致同一 grain 真的双活：
      registerToCluster 存的 value 是 ref.GetDirectAddr()，而 cluster ref 的 id 是
      "cluster/kind/name"（无 @addr）→ value 恒为 ""。抢锁失败的 B 在 start() 里走 stop()，
      stop() 的 defer 无条件调 unRegisterFromCluster → removeTxn(key, "") 比对 value=="" 恒成立
      → 删掉 A 正在持有的锁。此后任何节点再激活都会成功 → 两个实例同时存在、各自落盘。
      注意 setTxn 在 etcd 报错时也返回 false（provider_etcd.go:244），所以 B 只要 etcd 抖一下
      就会误删 A 的锁。修法：value 写节点地址（config.state.Address）而非 GetDirectAddr()，
      并且只在「本次注册确实成功」时才 unregister。
