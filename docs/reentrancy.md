# Grain 重入(Reentrancy)机制详解

> 目标读者:想读懂 `processor_mailbox.go` 里 turn / drainState / inflight 那套
> 逻辑的人。本文自底向上讲清楚"为什么需要"和"怎么工作"。

---

## 一、要解决的问题:同步 Ask 的自死锁

Actor 模型的铁律:**一个 actor 同一时刻只有一个 goroutine 在执行它的代码**
(Started / Receive / PreStop),这样 actor 内部状态无需加锁。

但框架提供了**同步** `Ask`:

```go
func (x *MyActor) Receive(ctx Context) {
    resp, err := ctx.Ask(otherActor, req)  // 阻塞,等 otherActor 回复
    use(resp)
}
```

`Ask` 会**阻塞当前 goroutine** 等回复。问题来了——回环调用:

```
A.Receive 里 Ask(B)  ── A 的 goroutine 阻塞,等 B 回复
   └─ B.Receive 里 Ask(A) ── B 要问 A
        └─ 但 A 的 goroutine 还卡在上面等 B... 谁来处理 B 发给 A 的消息?
```

**死锁**。A 在等 B,B 在等 A,而 A 唯一的执行 goroutine 被第一个 Ask 占住了,
没人能处理"B 问 A"这条消息。

**重入 = 让 A 在阻塞等待期间,腾出执行权去处理别的消息(包括 B 回问 A 的),
拿到回复后再继续。** 且全程仍保持"同时只有一个 goroutine 在跑 A 的代码"。

---

## 二、核心道具:turn(执行令牌)

```go
turn chan struct{}   // 容量 1 的信号量,初始放 1 个令牌
```

**规则:谁持有令牌,谁才有权执行 actor 代码。** 因为容量 1,任意时刻至多一个
goroutine 持有 → 严格单线程。

```go
func acquireTurn() { <-x.turn }          // 取令牌(取不到就阻塞等)
func releaseTurn() { x.turn <- struct{}{} } // 还令牌
```

死锁的破解思路:**Ask 阻塞前,先把令牌还回去**,让别的 goroutine 能拿令牌
进来处理消息;拿到回复后,再重新抢回令牌继续。

---

## 三、调度骨架:drainer goroutine + 状态机

在讲重入前,先理解正常(无 Ask)时消息怎么被处理。

### procStatus:调度器停车状态
```
idle    —— 没有 drainer 在跑,mailbox 空着
running —— 有一个 drainer goroutine 在排空 mailbox
stopped —— actor 已停
```

### schedule / process / run 三层

```go
send(msg):  rb.Push(msg); schedule()      // 投递 + 唤醒调度

schedule(): CAS(idle→running) 成功 → go process()   // 没在跑才起 goroutine
                                                     // 已在跑则 no-op(合并)

process(): ds := &drainState{}; run(ds); ...置回 idle + 复查

run(ds):   for {                          // 排空循环(drainer 主体)
             msg, ok := rb.Pop()
             if !ok { return }             // 空了,退出
             inflight++
             acquireTurn()                 // 抢令牌
             invoke(msg)                   // 执行 Receive(可能内部 yield/resume)
             releaseTurn()                 // 还令牌
             inflight--
           }
```

**关键点**:
- 持续有消息时,一个 drainer goroutine 在 `run` 循环里反复 Pop+处理,
  `schedule` 的 CAS 会失败(已 running),**不会每条消息起新 goroutine**。
- `drainState{}` 是**每个 drainer goroutine 独有的一面小旗**,只有一个字段
  `handedOff bool`,含义:"我这个 drainer 是否已经把排空职责交接给了后继"。

---

## 四、重入发生时:yield → 后继接管 → resume

现在看 A 在 Receive 里 `ctx.Ask(B)`。Ask 内部(`BaseActor.Ask`)关键三步:

```go
ds := x.turn.yieldTurn()                         // ① 让出令牌 + 派后继
sys.getSender().tellWithSender(target, msg, ...) // ② 发消息给 B
v, err := awaitReply(ch, timeout)                // ③ 阻塞等回复
x.turn.resumeTurn(ds)                            // ④ 抢回令牌
```

### ① yieldTurn:让出令牌,并"派一个接班的"

```go
func yieldTurn() *drainState {
    ds := x.activeDS                 // 当前持令牌者的 drainState(就是"我")
    if ds != nil && !ds.handedOff && x.life != lifeStarting {
        ds.handedOff = true          // 标记:我已交班
        go x.process()               // 派一个"后继 drainer"接管排空
    }
    x.releaseTurn()                  // 还令牌 → 后继或别人能拿了
    return ds
}
```

发生了什么:
- 我(A 的当前 drainer)即将阻塞在 Ask,不能再排空 mailbox 了。
- 所以**首次 yield 时** spawn 一个"后继 drainer"(`go x.process()`)——它会
  跑一个新的 `run` 循环,继续从 mailbox 取消息处理。
- `handedOff=true` 记住"我已交班",后面我 Ask 回来后不再负责排空(避免两个
  drainer 抢着排空)。
- 还令牌 → 后继 drainer 就能 `acquireTurn` 进去处理"B 回问 A"那条消息了。

> `activeDS` 是个中转变量:令牌持有者进 `run` 循环时会 `x.activeDS = ds`
> 写成自己的;`yieldTurn` 通过它拿到"当前是谁在持令牌"。不变量:**谁持令牌,
> activeDS 就是谁的 ds**(每次 acquire 后立即设置)。

### ②③ 发消息 + 阻塞等回复

令牌已让出,A 的这个 goroutine 现在阻塞在 `awaitReply` 的 channel 上。
**此时后继 drainer 正持令牌在跑**——它 Pop 到"B 问 A"的消息,执行 A 的
Receive(单线程!因为它持令牌,而阻塞中的我不持令牌),A 回复 B,B 的 Ask
返回,B 回复 A → 我的 `awaitReply` 收到回复,解除阻塞。

### ④ resumeTurn:抢回令牌继续

```go
func resumeTurn(ds *drainState) {
    x.acquireTurn()      // 重新抢令牌(要等后继让出/park)
    x.activeDS = ds      // 令牌又归我,activeDS 设回我的 ds
}
```

抢回令牌后,`use(resp)` 那些后续代码继续单线程执行。等 A 的 Receive 整个
返回,回到 `run` 循环(见下)。

---

## 五、drainer 退出时的交接判断

`run` 循环里 `invoke` 返回后:

```go
releaseTurn()
remaining := inflight.Add(-1)
if ds.handedOff {          // 我在这次 invoke 里 yield 过(交过班)
    if remaining == 0 && poisoned { schedule() }  // 停机兜底(见第七节)
    return                 // 我退出——排空职责已在后继身上,不能再抢
}
// 否则(没交过班):继续 for 循环 Pop 下一条
```

**为什么 handedOff 就要 return?** 因为 yield 时已经 spawn 了后继 drainer 接管
排空。如果我 resume 后还继续 `run` 循环,就会有**两个 drainer 同时排空**同一个
mailbox → 违反单线程。所以交过班的 drainer,处理完自己手上这条(resume 后
跑完的)就退出,把舞台留给后继。

`process()` 里对应:
```go
run(ds)
if ds.handedOff { return }   // 交过班,直接退,不碰 procStatus(后继拥有 running 角色)
// 没交班才 CAS(running→idle) 停车 + 复查队列
```

**"running-owner 角色"永远有交接**:一个 drainer 要么没交班、自己 CAS 回 idle;
要么交了班、后继继承 running。归纳下来不会出现"所有 drainer 都退了但 procStatus
卡在 running"。

---

## 六、通用重入语义(重要)

注意后继 drainer 是从 mailbox **按顺序取下一条**处理,**不限于"B 回问 A"那条**。
也就是说:A 阻塞在 Ask 期间,mailbox 里排在前面的**任意消息**都可能被后继处理。

这就是 **Akka/Orleans 式"通用可重入"**语义,代价:
> 一次 Receive 内(Ask 前后),actor 状态可能被**插入执行的其它消息**改变。
> 例:A 在 `x := a.field; ctx.Ask(...); use(a.field)` 里,两次读 `a.field`
> 可能不同——因为 Ask 期间别的消息改了它。

但**单线程保证仍成立**(同一时刻只有令牌持有者在跑),所以没有数据竞争,只是
状态可能"中途变化"。这是同步 Ask 换取"不死锁"的固有取舍。

---

## 七、几个配套字段(为什么存在)

| 字段 | 作用 | 为什么这样 |
|---|---|---|
| `inflight atomic.Int32` | 计"处理中的 handler 数",**含阻塞在 Ask 里的** | 停机时必须等所有 handler(包括挂起的)结束才能 stop;turn 只知道"谁在跑",不知道"谁挂着" |
| `poisoned atomic.Bool` | "已请求停止"的单调闩 | 与 procStatus 正交:actor 可"running 且 poisoned",闩要跨 idle↔running 存活 |
| `life lifecycle` | Started/PreStop 阶段 | `yieldTurn` 里 `life==lifeStarting` 时**不派后继**——Started 内的 Ask 不能让业务消息在 Started 完成前插进来 |
| `activeDS *drainState` | 当前持令牌者的 ds(中转) | 让 `yieldTurn`(经接口从 BaseActor 调过来)能拿到"当前 drainer 是谁" |

**停机与重入的交互**(第五节那个 `remaining==0 && poisoned` 兜底):
被 poison 后,阻塞在 Ask 的 handler 让 `inflight>0`,`run` 里 Pop 空也不能 stop
(得等挂起的 Ask 结束)。当最后一个挂起 handler resume 完、`inflight` 归 0 时,
就靠这行 `schedule()` 重新唤醒去跑停机检查(`doStop`,它会在持令牌 + inflight==0
时才真正 stop)。

---

## 七'、poison 的三重保险:为什么 `process` 里要 `|| (poisoned && inflight==0)`

停机(poison)的处理散落在三个地方,它们不是重复,而是覆盖三种不同时序。核心难点:
**`poison()` 只做两件事——`poisoned.Store(true)` + `schedule()`,而 `schedule()`
靠 `CAS(idle→running)` 唤醒 drainer;一旦这个 CAS 失败(当时是 running),就没起
新 drainer,poison 有可能"没人管"。** 三处保险就是堵各种"没人管"的洞。

```go
// 位置①  run 循环内 Pop 空:
if x.poisoned.Load() && x.inflight.Load() == 0 { x.doStop() }

// 位置②  run 里交过班的 drainer 退出前:
if remaining == 0 && x.poisoned.Load() { x.schedule() }

// 位置③  process 收尾复查(本节重点):
if x.rb.Len() > 0 || (x.poisoned.Load() && x.inflight.Load() == 0) { x.schedule() }
```

### 位置③后半段专治的竞态:poison 撞上 drainer 停车

时序(drainer 正要退出、poison 恰好这时到):

```
drainer(process→run):
  Pop() → 空;检查 poisoned → 此刻还是 false(poison 还没到);run 返回
  ┌──────────── 竞态窗口 ────────────┐
  │ 另一 goroutine: poison()          │
  │   poisoned.Store(true)            │
  │   schedule(): CAS(idle→running)   │
  └───────────────────────────────────┘
  process: CAS(running→idle)          ← 停车
  process: 第 203 行复查
```

看 `poison` 的 `schedule` 与 `process` 的 `CAS(running→idle)` 的相对顺序:

- **情况 A**:process 先把状态置回 idle,poison 的 `schedule` 才 CAS —— 看到 idle,
  CAS 成功,起新 drainer → 新 drainer Pop 空 → 见 poisoned → doStop。✓ 不需要后半段。
- **情况 B**(后半段专治):poison 的 `schedule` 先跑,但此刻 process **还没** CAS,
  procStatus 仍是 `running` → `CAS(idle→running)` **失败,no-op,不起 drainer**。
  随后 process 才 `CAS(running→idle)` 成功。结果:**poisoned=true、procStatus=idle、
  没有任何 drainer 在跑、也没人会再被唤醒**。若 process 只看 `rb.Len()>0`(队列空,
  false),就直接结束 → **poison 永久丢失,actor 僵死。**

**后半段 `poisoned && inflight==0` 就是让 process 在自己置 idle 后,除了复查队列还
复查 poison;发现有未处理的 poison 就自己 `schedule()` 再起一轮(此时 procStatus
已是 idle,CAS 必成功)去跑 doStop。**

### 为什么后半段要带 `&& inflight==0`(不能只 `|| poisoned`)

否则会 **busy-spin**:poison 来时有 handler 阻塞在 Ask(`inflight>0`)、队列空——
process 见 poisoned → schedule → 新 drainer → Pop 空 → 但 `inflight>0` 停不了 →
run 返回 → process 又见 poisoned → 又 schedule……**满核空转**直到 Ask 超时。
加 `&& inflight==0` 后,inflight>0 时 process 不 reschedule(反正现在也停不了);
真正的停止改由**位置②**触发——最后一个挂起 handler resume 完、`inflight` 归 0
且 poisoned 时,由它 `schedule()` 唤醒去跑 doStop。

### 三处各管什么(一句话)

| 位置 | 覆盖场景 |
|---|---|
| ① run 内 Pop 空 | drainer 还在跑,发现 poison 且没挂起 handler → 直接停 |
| ② 交班 drainer 退出前 | 挂起在 Ask 的 handler 归零那一刻 → 唤醒去停(治 busy-spin 的另一半) |
| ③ process 收尾复查 | poison 的 schedule 因 CAS 竞态失败 → 停车者自查补一枪(否则 poison 丢失) |

而 `doStop` 自己还会**持令牌后再复查一次** `rb.Len()!=0 || inflight!=0`(位置④):
从"run 决定停"到"doStop 抢到令牌"之间若又有 send 入队/新 handler,doStop 放弃这次,
交给后续 drainer 重试。四道检查合起来保证:**poison 不丢、不空转、不误停(还有活时)**。

---


## 八、完整时序图:a → b → a 回环

```
[drainer-A1]  run: Pop(msgX) → acquireTurn ✓ → A.Receive(msgX)
                                                   │ ctx.Ask(B, req)
                                                   │  ├ yieldTurn:
                                                   │  │    handedOff=true
                                                   │  │    go process() ──────► [drainer-A2] 启动
                                                   │  │    releaseTurn ✗令牌放出      │
                                                   │  └ 发 req 给 B, 阻塞等回复        │
                                                   │  (A1 goroutine 挂起,不持令牌)   │
                                                   │                          run: Pop(B问A的msg)
                                                   │                               acquireTurn ✓
                                                   │                               A.Receive(msg): 回复 B
                                                   │                               releaseTurn
              B 处理完 req → 回复 A ─────────────────┤                               (继续 Pop...)
                                                   │  awaitReply 收到回复
                                                   │  ├ resumeTurn: acquireTurn(等A2让出)✓
                                                   │  └ use(resp)
                                                   │ A.Receive(msgX) 返回
              releaseTurn; inflight--
              ds.handedOff==true → A1 退出(舞台留给 A2)
```

关键:任意瞬间,acquireTurn ✓ 的只有一个 goroutine → 单线程不破。A1 阻塞时不持
令牌,所以 A2 能进来处理"B 问 A",破解死锁。

---

## 九、一句话总结

> **重入 = Ask 阻塞前"让出执行令牌 + 派一个后继 goroutine 接管排空",
> 阻塞期间后继单线程处理其它消息(含回环消息),Ask 拿到回复后抢回令牌继续;
> 令牌容量 1 保证全程单线程,inflight 保证停机等所有挂起 handler 结束。**

3 个关键字段一句话记忆:
- `turn`:执行权(谁持有谁能跑,容量 1 = 单线程)
- `drainState.handedOff`:我这个 drainer 交过班没(交了就退,别抢排空)
- `inflight`:还有几个 handler 没结束(含挂起的,停机要等它归零)
