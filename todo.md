# 剩余待办（供你放进新 session）

  已确认的 bug，未修
  1. provider_etcd.go node-id 线性扫描：第 N 个加入的节点要 N 次 etcd 事务（每次 1 个 RTT），
     200 节点同时起 = sum(1..200) ≈ 2 万次。已核对无误。补：扫描上限是 uuid.MaxNodeMax()=1023，
     第 1024 个节点直接注册失败。
  2. 关停顺序：集群注册在排空 actor 之后才摘除（system_life.go:74 排空 → :76 摘注册）。
     【时间窗被低估一倍】不是 stopWaitTimeSecond：stopActorsImpl 的循环首轮不 sleep、
     之后每轮 sleep 1s、times>=N 才 break ⇒ 单次 (N-1) 秒；而它被调用两次(first/latter)
     ⇒ 默认 2×(3-1) = 4 秒。另：draining 只拦 ensureClusterKindActorExist 的按需激活，
     不拦发给已存在 actor 的消息，所以这 4 秒里消息照常投递进正在停止的 mailbox → 丢弃/dead letter。
     （你之前说跳过）

  性能，B 组（都动并发核心，建议每项单独一轮 + 压测 + race）
  3. Ask 每消息新建 goroutine（newproc 占 11.3% CPU，−300~400ns / −2 allocs）—— 需先给 ringbuffer 加无锁 size 原子计数器
  4. Ask future 池化（−3 allocs / −176B / −150ns）—— 回收条件必须是「cancelAsk 自己 Pop 成功」，否则 use-after-recycle
  5. turn 改为每批 drain 收放一次（−35ns/消息）
  6. addr_hash 负载不均。【数字要带节点数，原来的区间不完整】a203a96 只是把 hash/fnv 内联，
     算法仍是 fnv32a，测量未过期。1M key 实测偏离均值：
       10 节点 +11.2%/-16.5% → 加 fmix32 后 +0.3%/-0.4%
       50 节点 +14.6%/-8.9%  → 加 fmix32 后 +1.3%/-2.0%
      200 节点 +23.1%/-15.3% → 加 fmix32 后 +3.9%/-3.9%
     即：不均随节点数变差（200 节点 max/min 差 45%）；"<1%" 只在 ~10 节点成立，200 节点约 ±4%。
     "改 key→owner 映射、需协调重启" 无误；另外 addr_hash.go:25-27 的
     TestCalcAddrMatchesReferenceImplementation 钉死了与 hash/fnv.New32a 位级一致，改 hash 要同步改它。
  7. 远端发送 allocs。【不是 3，分两种路径】实测（复刻 stream_write.go:99-121，
     用接口调用强制 Envelope 逃逸，与交给 grpc Send 一致）：
       remote Tell (sender nil)     166 ns/op  2 allocs  168 B —— Marshal content 24B + Envelope 结构体 144B
       remote Ask  (replyRef sender) 263 ns/op  4 allocs  232 B —— 多出 replyRef.GetId() 的 FormatUint + 五段拼接
     string(proto.MessageName(msg)) 是 0 alloc（FullName 本身就是 string）。
     Envelope 结构体 144B 比 payload 还大，是这条路径最大的一块。
     复用 Envelope 前须确认 grpc stream.Send 返回后不再持有它；MarshalAppend 需配 per-actor buffer。
  8. askId 与 addr 伪共享同一 cache line（2 行改动）

  日志库（我测完了但没来得及汇报）
  9. 每条记录：zerolog 118ns / phuslu 179ns / zap 460ns / slog 704ns；With 强制逃逸后 zerolog 205ns+1alloc vs slog 351ns+6allocs。但 slog 是标准库、Logger() 已是懒构建、且被过滤的日志只要 10ns。换库要动公开
  API（Logger() *slog.Logger）—— 值不值得取决于你的日志量级

  设计 / 运维
  11. concurrentmap.go 死代码。【行数已变】现在是 420 行（不是 396），死代码约 145-160 行
      （≈35-38%）。关键补充：这些死代码全是**导出 API**（Upsert/SetIfAbsent/Has/RemoveCb/
      IsEmpty/Clear/Items/Keys/MarshalJSON/UnmarshalJSON…），删掉对 al/safemap 的外部使用者
      是 breaking change —— 先定这个包算不算公开 API。
  14. 【已验证：乒乓不会发生】score = fnv32a(name, addr) 只依赖 (name,地址)、与节点视图无关，
      且每个节点必然能看到自己（register 的 put 先于 watch 的 Get），所以每一跳在 (score,-addr)
      上严格单调 → 不可能成环；20 万次随机成员视图穷举 0 环，最长 5 跳（12 节点）。
      Envelope 里确实没有 hop/TTL 字段，但这个不变量目前没有任何测试或注释保护 ——
      一旦 hash 改成带权重或依赖视图，就没有兜底了。
  14b.【已修】grain 注册锁被抢锁失败方误删导致双活 —— value 改写 config.state.Address，
      并加 registered 标记只在注册成功时才 unregister。三个回归测试均已验证在旧代码上失败。
