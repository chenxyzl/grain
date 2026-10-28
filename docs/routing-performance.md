# 回复与远程路由优化

## 本地回复

Context.Reply 对同系统、本地地址的 replyRef 直接调用 deliverReply，省去通用路由分支。pending.Pop、非阻塞回复、重复/迟到回复处理及 shutdown 协议保持不变。跨系统转发、远程回复、普通 actor sender 和自定义发送器继续走原有路由。

## 集群变更与缓存边界

`routingResolver.resolve` 是统一解析入口，一次解析只使用一个 routingGeneration，内含节点快照、与快照配对的版本及 stream 引用表。

| 状态 | 保存位置 | 失效方式 |
|---|---|---|
| actor 完整路由（地址 + stream 引用） | ActorRef 的小型 actorRouteCache | resolver 或节点快照版本变化后重新计算，空地址也缓存 |
| 节点快照及 stream 引用 | system 的 routingResolver | 新快照发布后替换整个 generation，旧 generation 由 GC 回收 |

GetNodesVersion 只作为失效提示；generation 必须使用 GetNodes 返回的版本，避免版本提示领先快照时，把旧地址错误地标记为新版本。并发解析可以完成各自已经取得的快照，但旧解析不能覆盖同 resolver 的较新 actor 归属缓存。

ActorRef 不再执行选址；命中时直接复用完整路由，不再查询 generation 的 stream 表，也不持有 generation，避免大量 actor 引用保留整代节点及 stream 表。同一 ActorRef 经不同 system 发送时，缓存用 resolver 身份区分，不能仅比较两边恰好相同的版本号。

stream 表不保存 processor。每次远程发送仍从 registry 查找当前实例，同地址 stream 重建不会使用旧 processor。本地 Direct 和已有本地 processor 继续走快速路径，不读取节点快照。

与原有路由一致，归属变更依赖节点通知传播；这不是对已经在途消息的自动重试机制。Direct ActorRef 的显式地址也不会被改写为另一个节点。

## 合并 resolver 前的优化测量

环境：Go 1.27，linux/amd64，i7-10700KF。不开性能剖析，不与构建或其他 benchmark 并行，各三次取中位数。

| 场景 | 修改前 | 修改后 |
|---|---:|---:|
| BenchmarkAskMore，32 P | 547.8 ns/op | 361.4 ns/op |
| 串行本地 Ask，16 P | 3665 ns/op | 1500 ns/op |
| A→B→A 重入回环，16 P | 13434 ns/op | 12172 ns/op |
| stream 稳态路由，16 P | 247.7 ns/op | 110.2 ns/op |
| stream 路由分配 | 256 B/op，3 次 | 80 B/op，1 次 |

AskMore 使用修改前后独立二进制交错运行，耗时降低约 34%，均零丢信。stream 对照计入缓存查询、节点版本检查和 registry 查询，不包含 mailbox 执行、protobuf 序列化和网络 I/O，不能解读为端到端远程延迟降低 56%。版本切换和缓存首次填充也不在稳态数据内。

```bash
go test . -run '^$' -bench '^BenchmarkStreamRouting$' -benchmem -count=3
# 需要本地 etcd 127.0.0.1:2379
(cd examples && go test ./benchmark_test/actor_test -run '^$' -bench '^BenchmarkAskMore$' -benchmem -count=3)
```

## 验证

全量测试、全量 race、vet、根模块与 examples 构建通过。新增路由测试通过 race 20 轮，覆盖：同一 cluster ActorRef 的远程 A→远程 B→本地迁移、无 owner 后节点重新加入、并发节点版本变化、同地址 processor 替换、本地重复回复、自定义 sender 及跨系统回复。

## 统一解析验证

新增回归覆盖版本提示领先快照、旧解析晚完成、同一 ActorRef 跨 system 使用，以及 Direct 地址不随集群归属改变。保留迁移、无 owner 后重加入、stream processor 替换及并发节点变化测试。

统一 resolver 后，全量测试、全量 race、vet、根模块及 examples 构建通过，路由相关 race 回归通过 100 轮。

同机交错运行修改前后二进制，各三轮取中位数：

| 场景 | 合并前 | 合并后 |
|---|---:|---:|
| stream 缓存路由 | 105.8 ns/op | 113.5 ns/op |
| 单条 Tell 完成 | 704.9 ns/op | 706.8 ns/op |

分配分别保持 80 B/op 和 120 B/op。新版 stream 微基准通过统一入口读取目标类型及地址，旧版直接传入已经得到的地址；约 7.7ns 的差值不能当作完全同口径的缓存操作退化。三轮仍有波动，不宣称本次职责合并提升吞吐。节点变更和冷引用 benchmark 现在调用完整 resolve，包含需要时的 stream 引用获取，和旧版仅计算归属地址的结果不能直接比较。

## 完整路由缓存优化

此前每次命中 actor 地址缓存后仍执行一次 stream 表查询。现在缓存完整 resolvedRoute（地址 + stream 引用），且只在未命中时进行选址、stream 表查询和缓存写入；正常命中只有类型/本地判断、节点版本与原子缓存检查。缓存不持有 processor，stream 实例替换继续由 registry 处理。

新增 resolver-only benchmark 将解析工作与 registry 查询、Context 分配分开，避免把整个发送路由阶段误当成 resolver 自身耗时。

同机 16 P，修改前后独立二进制交错测量，各三轮、每轮 500ms，取中位数：

| 场景 | 修改前 | 修改后 | 分配变化 |
|---|---:|---:|---|
| Direct 稳态 resolver | 31.02 ns | 9.414 ns | 均 0 B/op |
| Cluster 稳态 resolver | 37.58 ns | 7.709 ns | 均 0 B/op |
| 缓存远程路由（含 registry、Context） | 119.7 ns | 96.54 ns | 均 80 B/op，1 次 |
| 每次新建 Cluster 引用并解析 | 393.2 ns | 383.8 ns | 192 → 208 B/op，均 3 次 |
| 节点变更并重新解析 | 3539 ns | 3383 ns | 1601 → 1617 B/op，均 12 次 |

冷路径的小幅耗时差异可能来自测量波动，不据此宣称加速。内存代价明确：Cluster 缓存条目从 32 B 增至 48 B；远程 Direct 引用首次解析也会创建一个 48 B 条目。引用重复使用时没有逐次分配。本地 Direct 快速路径不创建条目。不同节点版本下重新解析会替换旧条目。

全量测试、全量 race、vet、根模块与 examples 构建通过；路由相关 race 回归 100 轮通过。新增同一共享引用在并发节点变更下的验证，确保缓存的地址与 stream 来自同一代，最终收敛到最新节点版本。

```bash
go test . -run '^$' -bench '^BenchmarkRoutingResolver$' -benchmem -count=3
```
