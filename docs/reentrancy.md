# Grain 重入与按需调度

## 普通消息与重入

actor 使用按需排空的 drainer：有消息时启动，同一个 goroutine 连续执行 Receive，邮箱空后退出。没有独立调度 goroutine、逐消息执行许可事件、恢复队列或 worker 池。

```text
普通消息：Pop → 取得执行许可 → Receive → 归还许可 → Pop 下一条
Ask：让出许可并首次交接排空职责 → 发送/等回复 → 重新取得许可 → 继续 handler
```

同步 Ask 支持 self-Ask、A→B→A 和通用重入。等待期间，不相关消息也可以插入执行，因此 Ask 前后 actor 状态可能变化。同一时刻仍只有一个持有执行许可的 goroutine 运行 actor 代码。

## 状态与职责

| 主体 | 权威状态 |
|---|---|
| Mailbox | 消息 FIFO、容量、关闭状态 |
| Processor | actor 实例，连接 mailbox、scheduler、lifecycle |
| `mailboxScheduler` | `drainStatus`、`unfinishedCalls`，内嵌执行 gate |
| `executionGate` | `executionPermit`、`executionOwnerID`、`executingCall`，统一取得/释放执行权与身份校验 |
| `actorLifecycleState` | `lifecycleState`、`stopRequested`、`clusterRegistrationOwned` |
| drainer 私有 `execution` | `callKind`、`drainOwnershipTransferred`、执行作用域 |
| Context | 消息、sender、target、Reply/Forward |
| 系统 pending | 请求 ID → 回复等待对象，关闭后的注册禁令 |

scheduler 是负责调度的方法和状态集合，不是单独运行的协程。
`drainStatus` 为 idle/running/stopped，表示排空职责；不能据此判断是否还有挂起 handler。
`unfinishedCalls` 计数已出队但未完成的调用，包括初始化消息、等待执行许可、等待回复和正在执行的 handler。PreStop 在持有执行许可且该计数为零时单独运行。

调度状态与调用计数由多个 drainer 通过 scheduler 方法更新，需要原子操作。生命周期阶段和注册归属只在执行许可保护下访问；停止请求来自任意发送方，使用独立原子门闩。drainer 的交班标记只由其所属 goroutine 修改。

## Ask 交班协议

1. BaseActor 检查当前 goroutine 属于这个 actor 的普通 handler。
2. 注册回复等待对象。关闭系统后不再允许新注册。
3. 暂停 actor 执行作用域。第一次 Ask 将 `drainOwnershipTransferred` 置为 true，启动后继 drainer，后继继承 running 排空职责；然后归还 `executionPermit`。
4. 发送请求并等待回复、错误或超时。回复直接进入 pending，不排业务 mailbox。
5. 无论成功、失败还是 panic，先清理 pending，再重新取得执行许可并恢复作用域。
6. 原 handler 完成后退出自己的 drainer，不再取下一条消息；后继负责继续排空和停车。

同一 handler 再次 Ask 时不重复交班。若后继已经停在 idle，新入队消息会通过常规 schedule 唤醒新的 drainer。

不存在 processor 全局的 activeDS。当前 goroutine 通过执行作用域找到自己的 execution，不需要共享指针中转。

## 停车与停止的唤醒

有三个必要的检查位置，统一封装在 mailboxScheduler：

- 排空时：停止已请求且 `unfinishedCalls == 0`，尝试持许可停止。
- drainer 停车时：CAS running→idle 后重新检查邮箱和停止条件，补上与 send/Poison 竞争的唤醒。
- 已交班的最后一个 handler 完成时：若停止已请求，重新 schedule，使已停车的后继不遗漏停止工作。

只在计数为零时因停止请求重新调度，避免大量挂起 Ask 期间空转。
真正停止前，取得许可后再次检查邮箱与计数。普通消息和挂起调用全部结束，才开始 PreStop。

drainer 的退出清理使用 defer。handler 调用 runtime.Goexit 时，也会归还许可、减少未完成计数，并由仍拥有排空职责的 drainer 完成停车/重调度。

## 生命周期与关闭

初始化消息在 processor 发布前入队，所以业务消息不能越过 Started。Started 每个 actor 实例执行一次，drainer 的反复启动不会重新初始化 actor。

Started 失败时不调用 PreStop，但已取得的集群注册仍清理。停止前关闭 mailbox 入队，PreStop 期间新消息立即进入 stopped 死信路径；关闭竞争中已经入队的消息也会被清理并报告死信。

Poison 是独立停止门闩，不受 mailbox 满载影响。真正停止时注销 registry，并按本实例是否拥有注册决定集群清理。停止未排空不提前撤销节点 lease 的现有保护保留。

系统 pending 注册持 `pendingRegistrationMu.RLock` 直到并发安全的 pending 表完成写入，允许多个 Ask 并行注册。shutdown 持同一把 RWMutex 的写锁关闭新 Ask 并快照已有等待者，随后在锁外唤醒。回复与取消只使用 pending 表的分片锁，不反向获取注册锁。新 Ask 返回 `message.CodeSystemStopping`。

## 执行身份与使用约束

每个 actor 的 scheduler 内嵌 `executionGate`，由 gate 保存原子 `executionOwnerID` 和普通指针 `executingCall`，不再维护全局作用域表。BaseActor 仅引用 gate，不依赖 scheduler；gate 的 `enter`/`leave` 将许可和身份作为一个协议维护。drainer 启动时读取一次 `routine.Goid()`；取得执行许可后先写指针、再发布 owner ID，让出许可前先撤销 ID、再清空指针。Ask 只有在当前 Goid 与 owner ID 匹配后才读取指针，因此非法调用者不会并发读取执行作用域。Ask 挂起时撤销身份，恢复许可后重新发布。

没有采用普通 ThreadLocal：它与 pprof 标签共用 runtime 字段，pprof.Do 可能覆盖身份。当前方案兼容 pprof.Do，不需要 routinex。

- 子 goroutine 或其他 actor 的 handler 调用 BaseActor.Ask：返回 `message.CodeAskInvalidContext`，不发送请求。
- Started、PreStop 调用：返回 `message.CodeAskNotRunning`。
- NoReentryAsk 供 main、HTTP handler 等非 actor 调用者使用，不释放 actor 许可。
- 用户 defer/recover 在重新取得执行许可后执行，不会在 Ask 异常时与其他 handler 并行访问 actor 状态。

## 边界与性能

mailbox 上限不限制挂起调用栈；没有增加可能阻断 A→B→A 的固定 handler 上限。Ask 超时只限制回复等待，之后仍要等待执行许可；长时间运行的 handler 无法被安全强杀。

恢复执行与新消息竞争同一个许可，不承诺跨 Ask 的完成顺序或显式恢复 FIFO。调度协程完全按需，不保留此前固定循环原型的空闲超时配置。

性能比较见 [scheduler-performance.md](scheduler-performance.md)。回归测试覆盖重复 Ask、异常/Goexit、满邮箱恢复、停车唤醒竞争、生命周期和执行作用域清理。
