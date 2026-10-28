# 回复与远程路由优化

## 本地回复

Context.Reply 对同系统、本地地址的 replyRef 直接调用 deliverReply，省去通用路由分支。pending.Pop、非阻塞回复、重复/迟到回复处理及 shutdown 协议保持不变。跨系统转发、远程回复、普通 actor sender 和自定义发送器继续走原有路由。

## 集群变更与缓存边界

这里有两种不同的缓存：

| 缓存 | 键与内容 | 更新方式 |
|---|---|---|
| actor 归属地址 | 每个 cluster ActorRef 的 owner 地址 | 节点版本变化后，根据新节点快照重新计算；空地址也按版本失效 |
| stream 引用 | 目标节点地址 → 本地 stream ActorRef | 每次远程发送检查节点版本，版本前进时切换整个缓存代次 |

stream 缓存不保存 cluster actor 的归属，也不保存 processor。发送仍按已解析的目标地址选择 stream，并通过 registry 查找当前实例，因此同地址的 stream 关闭、重新创建不会继续使用旧 processor。并发调用只允许缓存代次向前推进；旧代次失去引用后由 GC 回收。

与原有路由一致，归属变更依赖节点通知传播；这不是对已经在途消息的自动重试机制。Direct ActorRef 的显式地址也不会被改写为另一个节点。

## 测量

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
