# Grain[[英文文档]](https://https://github.com/chenxyzl/grain/tree/main//README.md)
- 默认分布式actor模型.
- 使用简单. (只依赖etcd)
- 高度可扩展.
- 高性能. (测试运行 examples/benchmark_test/actor_test)
- actor支持请求重入. (actor 在处理消息时可再发起 Ask,即使 a->b->a 回环也不死锁;actor 始终单线程)
- 支持发布订阅(本地和全局)
- 支持schedule

# 环境要求
- Go >= 1.27 (泛型方法 `BaseActor.Ask[T]` 的要求)
- 一个 etcd 集群 (用于成员发现 / 集群寻址)

# 安装
- go get github.com/chenxyzl/grain/...

# 消息 API 速览
- `ref.Tell(msg)` — 向 actor 单向发送(fire-and-forget,`ActorRef` 上的方法)。
- `x.Self()` — 在 actor 内部取自己的 `ActorRef`。所有「引用」性质的操作都经由它:
  `x.Self().Tell(m)`、`x.Self().GetId()`。`BaseActor` 不再嵌入 `ActorRef`,所以用户
  actor 本身不再「是」一个 `ActorRef`。
- `ctx.Reply(msg)` — 在 `Receive` 内回复当前请求。
- `x.Ask[T](target, msg)` — **可重入**的阻塞式请求/应答,在 `Receive` 内使用
  (`BaseActor` 上的方法)。等待期间 actor 让出执行权,以便处理其它消息(包括回环
  给自己的应答),拿到回复后再恢复——不死锁,且仍单线程。返回
  `(T, *message.ErrCode)`。`T` 是期望的应答类型,因无法从入参推导,必须显式写出。
- `grain.NoReentryAsk[T](target, msg)` — 供**非 actor** 调用者(如 `main`、客户端)
  使用的阻塞式请求/应答。**不可重入**——不要在 actor 的 `Receive`/`Started` 里调。
  返回 `(T, *message.ErrCode)`。

> 所有请求/应答的错误都以 `*message.ErrCode` 返回(nil 表示成功),不 panic;
> 运行期失败(超时 / 远端错误 / 类型不符)都在此归类。
>
> ⚠️ **不要修改返回的 `*message.ErrCode`。** 「actor not found」这类框架错误是共享的
> 预分配值,而 `ErrCode` 的字段是导出的 —— 写 `err.Des` 会污染全进程后续所有 `Ask`。
> 需要补充上下文请用 `err.Code` / `err.Des` 新建一个 `ErrCode`。
>
> 判断具体是哪种失败用 `errors.Is(err, message.CodeActorNotFound)` —— 只比较 code,
> 忽略描述文字,并且能穿透 wrap。要按多个 code 分支则用
> `switch code, _ := message.CodeOf(err); code`。

> ⚠️ **`x.Ask` 只允许在 actor「运行中」时调用** —— 即普通 handler 里。在 `Started()`
> 或 `PreStop()` 里调用会什么都不发送、立即返回 `message.CodeAskNotRunning` 错误。
> `Started()` 期间重入是关闭的(handler 不能跑在只初始化了一半的状态上),actor 无法
> 回应任何进来的请求;`PreStop()` 里阻塞会重入停止流程。若想启动即发起 Ask,请在
> `Started()` 里自投递一条消息,在处理该消息时再 Ask。`Tell` 在所有阶段都不受限制。
> 详见 docs/reentrancy.md §九。

# 配置项
传给 `NewSystem(clusterName, version, clusterUrls, opts...)`。

| 配置项 | 默认值 | 作用 |
| --- | --- | --- |
| `WithConfigKind(name, producer, opts...)` | — | 注册一个集群 actor kind |
| `WithConfigAskTimeout(d)` | `3s` | 阻塞式 `Ask` 等待应答的时长 |
| `WithConfigStopWaitTimeSecond(n)` | `3` | 关停时等待 actor 排空的秒数 |
| `WithConfigGrpcListenAddr(addr)` | `":0"` | 本节点 grpc 监听的 host:port。同时决定对端来连的地址: 显式指定的 host 原样对外公布,通配则公布首个内网 IP(没有内网网卡时回落到回环)。默认值表示端口由内核分配,因此同机可以起多个节点 |
| `WithConfigEtcdLeaseTTLSecond(n)` | `10` | 本节点 member key 所挂 lease 的 TTL —— 即节点崩溃(来不及注销)后对端仍会往这里路由的最坏时间窗 |
| `WithConfigEtcdDialTimeout(d)` | `10s` | etcd 首次连接、以及关停时 revoke lease 的超时 |
| `WithConfigGrpcDialOptions(...)` | insecure creds | **追加**对外 peer 流的 dial 选项 |
| `WithConfigGrpcCallOptions(...)` | — | 追加对外 peer 流的 call 选项 |
| `WithConfigLogger(l)` | `slog.Default()`,在 `Start()` 时读取 | system、cluster provider 和所有 actor 的日志都由它派生。用它可以摆脱 `InitLog` 的顺序要求 —— 见下 |
| `WithConfigDeadLetter(h)` | WARN 日志 | 无法投递消息(mailbox 溢出、发给已停止的 actor)的处理器。跑在发送方 goroutine 上,必须快 |

> ⚠️ **`InitLog` 对调用顺序敏感,`WithConfigLogger` 不敏感。** `InitLog` 调的是
> `slog.SetDefault`,而 system 是在 `Start()` 里读这个全局值来构造自己的 logger 的。
> 如果在 `Start()` **之后**才调 `InitLog`,你自己打的 slog 日志会切到新 handler,而框架
> 的日志会静默地继续走旧的。要么在 `Start()` 之前调 `InitLog`,要么干脆不碰全局:
> `grain.WithConfigLogger(grain.NewLogger("./game.log", slog.LevelInfo))`。

> NAT 后面或做了容器端口映射的节点,`WithConfigGrpcListenAddr` 覆盖不了 —— 真正可达的
> 地址进程自己看不到。单独的 advertise 地址目前还没有。

# 例子:

## examples/first(通知&请求应答)
注意: 先运行一个etcd
- 申明actor:
``` go file:hello_actor.go
package share_actor

import (
"examples/testpb"
"fmt"

"github.com/chenxyzl/grain"
"google.golang.org/protobuf/proto"
)

type HelloActor struct{ grain.BaseActor }

func (x *HelloActor) Started() { x.Logger().Info("Started") }
func (x *HelloActor) PreStop() { x.Logger().Info("PreStop") }
func (x *HelloActor) Receive(context grain.Context) {
switch msg := context.Message().(type) {
case *testpb.HelloAsk: //ask-reply
x.Logger().Info("recv ask", "message", context.Message())
context.Reply(&testpb.HelloReply{Name: "reply hello to " + context.Sender().GetName()})
case *testpb.Hello: //tell
x.Logger().Info("recv tell", "message", context.Message())
default:
panic(fmt.Sprintf("not register msg type, msgType:%v, msg:%v", proto.MessageName(msg), msg))
}
}
```

- 启动system:
``` go
package main

import (
"examples/share_actor"
"examples/testpb"

"github.com/chenxyzl/grain"
)

func main() {
//warning: etcd url
//create system
system := grain.NewSystem("hello_first", "0.0.1", []string{"127.0.0.1:2379"})
//start
system.Start()
//create a actor and return a actorRef
actorRef := system.Spawn(func() grain.IActor { return &share_actor.HelloActor{} })
//tell
actorRef.Tell(&testpb.Hello{Name: "hello tell"})
//ask
reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, &testpb.HelloAsk{Name: "hello ask"})
if err != nil {
panic(err)
}
system.Logger().Info("reply:", "message", reply)
//waiting ctrl+c
system.WaitStopSignal(nil, nil)
}
```
## examples/cluster(集群模式)
注意: 先运行一个etcd  
注意: 定义一个actor(和上面例子一样--略过)

- 集群服务器
``` go
package main

import (
"examples/share_actor"
"log/slog"

"github.com/chenxyzl/grain"
)

func main() {
grain.InitLog("./test.log", slog.LevelInfo)
//system
system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
grain.WithConfigKind("player", func() grain.IActor { return &share_actor.HelloActor{} }))
//start
system.Logger().Warn("system starting")
system.Start()
system.Logger().Warn("system started successfully")
//wait ctrl+c
system.WaitStopSignal(nil, nil)
//
system.Logger().Warn("system stopped successfully")
}

```
- 集群客户端
``` go
package main

import (
"examples/testpb"
"log/slog"
"strconv"
"time"

"github.com/chenxyzl/grain"
)

func main() {
grain.InitLog("./test.log", slog.LevelInfo)
//new system
system := grain.NewSystem("hello_cluster", "0.0.1", []string{"127.0.0.1:2379"},
grain.WithConfigAskTimeout(time.Second*1))
//start
system.Logger().Warn("system starting")
system.Start()
system.Logger().Warn("system started successfully")
//get a cluster actorRef
actorRef := system.GetClusterActorRef("player", "123456")
if actorRef == nil {
panic("GetClusterActorRef failed")
}
//
go func() {
c := time.NewTicker(3 * time.Second)
times := 0
for range c.C {
times++
//tell
actorRef.Tell(&testpb.Hello{Name: "hello tell, times:" + strconv.Itoa(times)})
//ask
system.Logger().Info("ask: ", "target", actorRef)
reply, err := grain.NoReentryAsk[*testpb.HelloReply](actorRef, &testpb.HelloAsk{Name: "xxx, times:" + strconv.Itoa(times)})
if err != nil {
system.Logger().Error(err.Error())
}
system.Logger().Info("reply:", "message", reply)
}
}()

//wait ctrl+c
system.WaitStopSignal(nil, nil)
//
system.Logger().Warn("system stopped successfully")
}

```

## examples/hello_reentry（请求重入）
注意: 先运行一个etcd

actor 在处理消息时可以 `x.Ask[T]` 另一个 actor。若被调方回头再问自己(a -> b -> a),
不会死锁:`A` 等待期间让出执行权,由后继 drainer 处理 `B` 回发给 `A` 的消息;
应答到达后 `A` 恢复。actor 始终单线程。
``` go
func (x *HelloActorA) Receive(ctx grain.Context) {
switch ctx.Message().(type) {
case *testpb.HelloAskB2A:
// A 正阻塞在 x.Ask(B) 时被 B 回问 —— 重入
ctx.Reply(&testpb.HelloReplyB2A{Name: "HelloReplyB2A"})
}
}

func (x *HelloActorB) Receive(ctx grain.Context) {
switch ctx.Message().(type) {
case *testpb.HelloAskA2B:
// 重入地回问 A,不会死锁
reply, err := x.Ask[*testpb.HelloReplyB2A](helloActorA, &testpb.HelloAskB2A{Name: "HelloAskB2A"})
_ = reply; _ = err
ctx.Reply(&testpb.HelloReplyA2B{Name: "HelloReplyA2B"})
}
}
```
> `x.Ask[T](...)`(`BaseActor` 上的方法,可在 `Receive` 内调用)是可重入形式。
> turn/交接机制的原理见 docs/reentrancy.md。

## examples/pubsub（发布订阅）
- 订阅事件  
$system.Subscribe(ref ActorRef, message proto.Message)
- 发布本地事件  
$system.PublishLocal(message proto.Message)
- 发布集群全局事件  
$system.PublishGlobal(message proto.Message)
- 取消订阅  
$system.Unsubscribe(ref ActorRef, message proto.Message)

## examples/schedule（延时调度）
- actor 延时调用一次  
$actor.ScheduleSelfOnce(delay time.Duration, msg proto.Message)
- system 延时调用一次  
$system.GetScheduler().ScheduleOnce($actorRef, `/*more params like above*/`)
- actor 延时重复调用  
$actor.ScheduleSelfRepeated(delay time.Duration, interval time.Duration, msg proto.Message)  
- system 延时重复调用
$system.GetScheduler().ScheduleRepeated($actorRef, `/*more params like above*/`)
- 取消延时调用
CancelScheduleFunc()

> ⚠️ **调度的消息是原样投递的,框架不会复制。** `ScheduleRepeated` 每次 tick 交给目标的
> 都是**同一个**实例,调用方自己也还持有它 —— 所以 handler 里写进去的字段下一个 tick
> 依然在;若目标是**远端** actor,并发写还会和 write-stream goroutine 上的 marshal
> 形成 data race。要么调度一个无字段的触发消息、在 handler 里再构造真正的消息,要么
> 修改前先 `proto.Clone`。


## 更多例子
更多例子参考： /examples

## Benchmark
> 需要本地 127.0.0.1:2379 上有一个 etcd(benchmark 的 NewSystem 连这里)。

编译 benchmark 可执行文件(`CGO_ENABLED=0` 得到静态、可交叉编译的二进制)
``` bash
  cd examples/benchmark_test/actor_test
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c -o bench-windows-amd64.exe .
```
运行(编出来的测试二进制用 `-test.` 前缀参数;`-test.run=none` 跳过普通测试,只跑 benchmark)
``` cmd
  bench-windows-amd64.exe -test.run=none -test.bench=. -test.benchmem
```
result
``` benchmark result
goos: windows
goarch: amd64
pkg: examples/benchmark_test/actor_test
cpu: Intel(R) Core(TM) i7-10700KF CPU @ 3.80GHz
BenchmarkSendOne-32              4953276               225.0 ns/op            80 B/op          1 allocs/op
BenchmarkSendMore-32            21939768                72.99 ns/op           80 B/op          1 allocs/op
BenchmarkAskOne-32                803352              1523 ns/op             273 B/op          5 allocs/op
BenchmarkAskMore-32              5265314               246.6 ns/op           273 B/op          5 allocs/op
PASS
```