# Grain 按需调度重构方案

状态：已按性能测量结果调整为按需排空方案。独立固定调度循环原型已撤回；保留职责拆分、明确命名和正确性修复。

## 目标

- 普通消息在同一个 drainer 中连续处理，避免逐消息跨调度器与 worker 交接。
- 保留同步 Ask、self-Ask、A→B→A 和通用重入。
- 状态有明确负责人，不要求每个负责人拥有独立 goroutine。
- 保留非法执行身份检查、panic 恢复许可、Goexit 清理和 shutdown 注册保护。
- 不引入固定 worker 池、routinex、自定义编译参数或空闲定时器。

## 组件边界

| 组件 | 文件 | 职责 |
|---|---|---|
| Processor | `processor_mailbox.go` | 绑定 actor、初始化队列、连接发送/调度/生命周期 |
| Mailbox | `al/ringbuffer/` | FIFO、容量、并发入队出队、关闭 |
| Mailbox scheduler | `processor_scheduler.go` | 按需启动、连续排空、未完成计数、停车补偿 |
| Actor lifecycle | `processor_lifecycle.go` | Started/PreStop、阶段推进、注册与注销归属 |
| Execution | `execution.go` | drainer 私有调用作用域、Ask 交班与恢复、异常处理 |
| Execution gate | `execution_gate.go` | scheduler 内嵌持有，统一管理执行许可、owner ID 和当前调用；BaseActor 只引用 gate |
| Context | `context_impl.go` | 消息元数据、Reply/Forward，不维护调度状态 |
| 回复与系统关闭 | `request_helper.go`、`system*.go` | pending 注册、等待、清理和关闭门闩 |

职责归属与线程归属分开：scheduler 的计数/停车状态由多个 goroutine 更新，因此保留原子同步。生命周期阶段和注册归属由 executionPermit 串行保护；执行作用域内的交班标记是 goroutine 私有数据。

## 命名与迁移

| 原命名或固定循环状态 | 当前名称/处理 |
|---|---|
| rb | mailbox |
| procStatus | drainStatus：idle/running/stopped 排空状态 |
| turn | executionPermit：运行 actor 代码的独占许可 |
| inflight | unfinishedCalls：已出队但尚未完成的调用 |
| handedOff | drainOwnershipTransferred：本 drainer 已交班 |
| life | lifecycleState：actor 生命周期 |
| poisoned | stopRequested：请求门闩，不代表停止已完成 |
| registered | clusterRegistrationOwned：本实例拥有成功取得的注册 |
| activeDS | executingCall：gate 当前持许可的执行作用域，必须先校验 executionOwnerID |
| controlEvents、resumeQueue、reusableCall、idleTimer | 删除，不再逐消息集中调度 |
| WithOptsSchedulerIdleTimeout | 撤回原型 API，空邮箱直接停车 |

## 执行与停止协议

普通路径为 Pop→取得许可→Receive→归还许可→下一条。每个 drainer 只读取一次 Goid，普通消息复用作用域；取得许可后发布执行身份，归还许可前撤销。BaseActor 在发布前绑定 executionGate，Ask 直接校验 owner ID 后读取 executingCall，不经过全局表。

第一次 Ask 启动后继 drainer 并移交排空职责；原 handler 归还许可后发送和等待，结束等待后重新取得许可。交班标记防止它完成后再次排空。同一 handler 后续 Ask 不重复交班。

Ask 用 defer 保证清理 pending 后再恢复执行权；用户 panic/recover/defer 不能在无许可时操作 actor 状态。所有 handler 退出路径，包括 Goexit，均完成许可归还和计数减少。

停车与 Poison 的补偿检查集中在 scheduler：排空检查、CAS 停车后的工作复查、最后一个已交班调用完成时的停止唤醒。停止前持许可复查，计数非零时不忙等。

初始化消息在 registry 发布前排在队首。Started 失败仍清理已取得的注册，PreStop 只与成功 Started 配对。进入 PreStop 前关闭入队；关闭竞争中已接受的消息必须产生死信，不能消失。

系统 pending 注册持读锁完成入口检查与插入；关闭持写锁完成关闭入口与等待者快照。注册可以并行，且不会在 shutdown 快照后插入遗留等待者。pending 表内部的分片锁负责并发写入，回复与取消不获取入口锁。

## 验证与性能

保留原有回归，并增加：重复 Ask、自环/回环、发送 panic 后用户 recover 的执行权、Goexit 后排空继续、满邮箱恢复、停车与 Poison 竞争、错误 actor/子协程拒绝、pprof 标签和作用域清理。

```bash
go test ./...
go test -race ./...
go test . -run 'Test(Scheduler|ExecutionOwner|ExecutionScope|AskPanic|AskRoundTrip|AskRepeated|PendingRegistration)' -count=50
go build ./...
go vet ./...
(cd examples && go build ./...)
go test . -run '^$' -bench 'Benchmark(Actor|AskLocal)' -benchmem -count=3
go test . -run '^$' -bench '^BenchmarkIdleActorFootprint$' -benchtime=1x
```

对比原始实现、固定循环原型和当前按需实现，使用相同完成路径 benchmark；不能用洪泛下的发送速度替代实际处理吞吐。结果见 `scheduler-performance.md`。

## 保留的限制

执行身份的原子发布/撤销、异常保护仍有成本，不承诺与最初版本逐纳秒相同。routine.Goid 依赖 runtime 内部布局，升级 Go 需兼容测试。

未完成调用没有硬上限；通用重入允许 Ask 前后状态变化；回复超时不保证恢复许可的等待上限。对外监控 API、调用级取消和更细重入准入策略不在本次范围内。
