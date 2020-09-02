# TOC

- [TOC](#toc)
- [传统crontab痛点](#传统crontab痛点)
- [Scheduler特点](#scheduler特点)
- [配置文件](#配置文件)
	- [Master配置](#master配置)
	- [Worker配置](#worker配置)
- [整体架构](#整体架构)
	- [Master](#master)
		- [JobManager](#jobmanager)
		- [LogManager](#logmanager)
		- [ApiServer](#apiserver)
		- [WorkerManager](#workermanager)
	- [Worker](#worker)
		- [JobManager](#jobmanager-1)
		- [Scheduler](#scheduler)
		- [Executor](#executor)
		- [JobLock](#joblock)
		- [LogSink](#logsink)
		- [Register](#register)
- [issues（遇到的问题）](#issues遇到的问题)
	- [分布式锁偏向](#分布式锁偏向)
		- [问题描述](#问题描述)
		- [解决方案](#解决方案)
	- [无法杀死任务](#无法杀死任务)
		- [分析](#分析)
		- [原因总结](#原因总结)
		- [解决方案](#解决方案-1)
# 传统crontab痛点
- 当机器故障时，任务会停止调度，甚至丢失crontab配置
- 任务数量多，单机的硬件资源不够，需要人工迁移到其他机器
- 需要人工去机器上配置cron，任务执行状态不方便查看

# Scheduler特点
- 分布式部署，当单机故障时，任务可以继续执行，任务保存在etcd中，不易丢失
- 分布式并发的调度任务，提升整体调度性能
- 可视化的Web界面，可以方便的 添加任务/删除任务/查看日志/强杀任务/...
- 实现了服务注册和发现，当任务数量过多时候可以平滑的增加机器分摊压力
# 配置文件
## Master配置
> master/main/master.json
```json
{
  "apiPort": 8070, //http端口
  "apiReadTimeOut": 5000, //http 读超时时间（单位ms）
  "apiWriteTimeout": 5000,//http 写超时时间（单位ms）
  "etcdEndpoints": ["120.79.182.28:2379"], //etcd集群地址
  "etcdDialTimeout": 5000, //etcd连接超时时间
  "webapp": "./webapp", //web前端界面路径
  "dbUri": "127.0.0.1:3306", //DB的地址
  "dbName" : "scheduler_log", //数据库库名
  "dbUsername" :"root", //账户
  "dbPassword": "admin" //密码
}
```

## Worker配置
> worker/main/worker.json
```json
{
  "etcdEndpoints": ["120.79.182.28:2379"], //etcd集群地址
  "etcdDialTimeout": 5000, //etcd连接超时时间
  "bashPath": "/usr/bin/bash", //bash路径
  "dbUri": "127.0.0.1:3306", //DB的地址
  "dbName" : "scheduler_log", //数据库库名
  "dbUsername" :"root", //DB账户
  "dbPassword": "admin", //DB密码
  "logChanLen" : 1000, //日志chan队列长度
  "jobLogBatchSize": 100, //日志提交批次大小
  "jobLogCommitTimeOut" : 1000 //日志批次超时时间
}
```
# 整体架构

整体分为两部分：`Master`节点和`Worker`节点
## Master
![mark](http://static.imlgw.top/blog/20200902/t4gAcODEDQzG.png?imageslim)
`Master`结构相对简单，主要作用就是对任务的增删改查以及日志监控，`Master`分为几个不同的模块，分别为`JobManager`任务管理，`LogManager`日志管理，`ApiServer` Web接口等
### JobManager
该模块主要功能就是和`Etcd`做交互，保存任务，删除任务，查询任务，以及发送强杀任务通知
### LogManager
该模块主要作用就是和`MySQL`交互，查询任务日志
### ApiServer
该模块主要功能就是提供Web服务，处理用户的web请求，解析参数，再通过上面的`Manager`模块，执行对应的方法
### WorkerManager
服务发现模块，主要作用就是拉取注册在Etcd中正常的Worker节点列表
## Worker
![mark](http://static.imlgw.top/blog/20200902/roVBByGGeGAo.png?imageslim)
`Worker`主要作用就是调度以及执行任务，`Worker`分为几个不同的模块，`JobManager`任务模块，`Scheduler`调度器模块，`Executor` 执行器模块，`JobLock`分布式锁，`LogSink`日志模块
### JobManager
该模块和`Master`的`JobManager`对应，该模块主要是监听`Master`节点对任务的增删改查以及强杀kill等操作，然后将这些操作封装成事件`Event`推送给调度器，让调度器`Scheduler`做出响应
### Scheduler
该模块就是核心的调度模块，负责处理前面`JobManager`推送过来的`Event`同步执行计划表，并且调度最近将要过期的任务将其交给`Executor`执行器，同时返回下一次将要过期的任务时间间隔，进行精确的睡眠，避免for循环导致cpu无意义空转
### Executor
执行器模块，在收到`Scheduler`调度器发送过来的任务时，会启动一条`goroutine`协程根据指定的bash去执行具体的任务，然后构建执行结果回传给`Scheduler`调度器，调度器会更新执行表，以及进行日志的落地
### JobLock
分布式锁模块，因为整个项目是在分布式的环境下建立的，所以会有多个`Worker`节点，每个`Worker`节点都有机会执行`etcd`中的任务，所以当某个任务到期的时候，可能存在多个节点同时执行同一个任务的情况，这肯定是不允许的，所以我们需要进行并发的控制，确保一个任务同一时间只能有一个节点执行，从而引入分布式锁

>我这里用`etcd`自己实现了一套我所需要的分布式锁，并没有使用etcd客户端中实现的分布式锁的轮子

在查看`etcd/clientv3/concurrency`中实现的分布式锁后，发现和我的需求不太一致，下面简单分析下etcd客户端的实现方式
```golang
//etcd v1.3.4
func (m *Mutex) Lock(ctx context.Context) error {
	s := m.s
	client := m.s.Client()

	m.myKey = fmt.Sprintf("%s%x", m.pfx, s.Lease())
	//比较加锁的key的修订版本是否是0。如果是0就代表这个锁不存在。
	cmp := v3.Compare(v3.CreateRevision(m.myKey), "=", 0)
	// put self in lock waiters via myKey; oldest waiter holds lock
	//向加锁的key中存储一个空值，这个操作就是一个加锁的操作，可以看出这里是没有锁竞争的问题的
	//这里租约的ttl就是session的过期时间，session创建后会自动给这个租约续租
	put := v3.OpPut(m.myKey, "", v3.WithLease(s.Lease()))
	// reuse key in case this session already holds the lock
	//获取当前节点的key
	get := v3.OpGet(m.myKey)
	// fetch current holder to complete uncontended path with only one RPC
	//获取当前锁的拥有者（通过prefix查询最小的revision）
	getOwner := v3.OpGet(m.pfx, v3.WithFirstCreate()...)
	//将上面的合并成一个事务操作
	resp, err := client.Txn(ctx).If(cmp).Then(put, getOwner).Else(get, getOwner).Commit()
	if err != nil {
		return err
	}
	m.myRev = resp.Header.Revision
	if !resp.Succeeded {
		m.myRev = resp.Responses[0].GetResponseRange().Kvs[0].CreateRevision
	}
	// if no key on prefix / the minimum rev is key, already hold the lock
	ownerKey := resp.Responses[1].GetResponseRange().Kvs
	//如果没有人持有锁，或者自己就是锁的持有者就直接返回了
	if len(ownerKey) == 0 || ownerKey[0].CreateRevision == m.myRev {
		m.hdr = resp.Header
		return nil
	}
	//否则说明锁被占用了，就要等前一个revision释放锁
	// wait for deletion revisions prior to myKey
	hdr, werr := waitDeletes(ctx, client, m.pfx, m.myRev-1)
	// release lock key if wait failed
	if werr != nil {
		m.Unlock(client.Ctx())
	} else {
		//获得锁
		m.hdr = hdr
	}
	return werr
}
```
再深入的看一下`waitDeletes`的实现，这里模拟了一种公平的先来后到的排队策略，等待所有小于当前revision的key都删除后锁释放才返回
```golang
// waitDeletes efficiently waits until all keys matching the prefix and no greater
// than the create revision.
func waitDeletes(ctx context.Context, client *v3.Client, pfx string, maxCreateRev int64) (*pb.ResponseHeader, error) {
	//注意这两个option，通过这两个option就可以获取当前rev的前一个key
	getOpts := append(v3.WithLastCreate(), v3.WithMaxCreateRev(maxCreateRev))
	for {
		resp, err := client.Get(ctx, pfx, getOpts...)
		if err != nil {
			return nil, err
		}
		//没有小于当前revision的key了，说明前一个以及被删除了，当前节点获取锁
		if len(resp.Kvs) == 0 {
			return resp.Header, nil
		}
		lastKey := string(resp.Kvs[0].Key)
		if err = waitDelete(ctx, client, lastKey, resp.Header.Revision); err != nil {
			return nil, err
		}
	}
}

func waitDelete(ctx context.Context, client *v3.Client, key string, rev int64) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wr v3.WatchResponse
	//watch前一个revision
	wch := client.Watch(cctx, key, v3.WithRev(rev))
	//wch是一个channel
	for wr = range wch {
		for _, ev := range wr.Events {
			//如果被删除则返回
			if ev.Type == mvccpb.DELETE {
				return nil
			}
		}
	}
	if err := wr.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("lost watcher waiting for delete")
}
```
Unlock删掉当前节点的key就行了
```golang
func (m *Mutex) Unlock(ctx context.Context) error {
	client := m.s.Client()
	if _, err := client.Delete(ctx, m.myKey); err != nil {
		return err
	}
	m.myKey = "\x00"
	m.myRev = -1
	return nil
}
```
这样的分布式锁的实现方式不存在锁的竞争，也没有重复的尝试加锁的操作，通过一个统一的前缀去put值，然后根据版本号的大小进行排队，先来后到，可以避免[惊群效应](https://zh.wikipedia.org/wiki/%E6%83%8A%E7%BE%A4%E9%97%AE%E9%A2%98)，因为这里每个节点只需要监听它前一个revision，所以当某个session释放锁的时候只会影响它的下一个节点，并不会惊醒其他的节点
> 如果熟悉Java的同学看到上面的时候肯定会想起啥，没错，这个的实现和[aqs](http://imlgw.top/2019/09/24/abstractqueuedsynchronizer/)很类似，或者说这个就像一个分布式的[clh队列](http://imlgw.top/2019/08/10/zi-xuan-suo-clh-suo-mcs-suo/)

那么为什么我不直接使用这个轮子呢？其实原因很简单
1. 上面实现的分布式锁在抢不到锁的时候需要**等待前面的节点释放锁**，而在我的这个应用中这种情况是不应该继续等待的，因为etcd中的任务不是只有这一个，还会有很多其他的任务，而各个任务之间是没有关联的，所以这里应该直接放弃抢锁结束当前协程，转而去争抢下一次将要执行的任务的锁，去执行其他的任务，这样才能充分的利用计算机资源，而不是“在一棵树上吊死”
2. 任务是根据cron表达式计算的调度时间，在某个任务到期需要执行的时候才会进行分布式锁的争抢，抢到锁的节点就执行任务，当这个锁被其他节点抢走意味着**此时/该次**任务的执行权已经被抢走了，这个时候其他的节点等待这个锁是没有意义的，甚至错误的，设想，如果最后等到了锁的释放，某个节点抢到了，那么它就会继续执行该任务，很明显这里就有问题，会重复的执行任务，所以我需要的分布式锁其实隐含着和**时间维度**是有关联的，也就是某个时刻某个任务执行所需要的锁
### LogSink
日志模块，当woker启动的时候就会初始化日志模块，同时启动一个协程去监听日志消息，调度器会将执行器执行的结果推送给日志，日志模块会通过gorm和MySQL交互，将日志存储在MySQL中

这里由于和MySQL交互性能开销会比较大，所以这里会将多条日志组合在一起提交，当chan中的log条数达到了我们初始设置的阈值就会自动提交，批量的插入DB，于此同时，每个批次在初始化的时候都带有一个定时器，当超过了我们配置的最大等待时间也会自动提交，这样做的好处就是可以减少网络传输的开销，同时也可以减少链接MySQL的次数（如果没有使用连接池的话）
### Register
服务注册模块，当Worker启动时候向Etcd中注册节点信息，并不断的续租，确认节点在线状态
# issues（遇到的问题）
## 分布式锁偏向
### 问题描述
在Worker节点通过分布式锁抢占任务执行权的时候由于时钟周期不一致，可能导致有的机器可能会一直快人一步获取到锁，进而导致某些机器过于繁忙，而有些机器过于空闲
### 解决方案
这里我采用了一种比较简单的解决方案，在获取锁之前让Worker随机的sleep一小段时间，这样各个Worker获取任务的机会就均等了
## 无法杀死任务
### 分析
> go version go1.13.6 linux/amd64

在实现kill强杀功能时候发现的问题，无法杀死任务，即使kill了还是会等到任务执行完才会返回，在查资料的过程中发现这应该也算是golang本身的一个坑了，参考[issue23019](https://github.com/golang/go/issues/23019)

一开始是在Windows平台上运行测试的，以为是平台的原因但是在切换了Linux后问题依然存在，如下poc既可复现

```golang
package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

func main() {
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelFn()
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", "sleep 120; echo hello")
	output, err := cmd.CombinedOutput()
	fmt.Printf("output：【%s】err:【%s】", string(output), err)
}
```
在linux平台上通过go run执行上面的代码，然后`pstree -p`查看进程树
```golang
//由于进程树比较庞大，所以省略了一些无关的部分
systemd(1)─┬─...
           ├─sshd(17395)─┬─sshd(18450)───sftp-server(18453)
           │             ├─sshd(28684)───zsh(28687)───go(2661)─┬─demo3_cmdPit(2680)─┬─bash(2683)───sleep(2684)
           │             │                                     │                    ├─{demo3_cmdPit}(2681)
           │             │                                     │                    ├─{demo3_cmdPit}(2682)
           │             │                                     │                    └─{demo3_cmdPit}(2686)
           │             │                                     ├─{go}(2662)
           │             │                                     ├─{go}(2663)
           │             │                                     ├─{go}(2664)
           │             │                                     ├─{go}(2665)
           │             │                                     ├─{go}(2672)
           │             │                                     ├─{go}(2679)
           │             │                                     └─{go}(2685)
           │             └─sshd(29042)───zsh(29044)───pstree(2688)
```
可以看到这里我们的go进程`demo3_cmdPit(2680)`创建了`bash(2683)`进程去执行具体的shell指令，但是这里由于我们执行的命令是【`sleep 50; echo hello`】属于一组多条命令，所以这里shell会fork出子进程去执行这些命令，所以上面显示`sleep(2684)`是`bash(2683)`的子进程
>补充：除了多条命令会产生子进程外，还有一些情况会产生子进程，比如`&`后台运行，`pipe`管道，外部shell脚本等等，具体可以参考这篇文章[子shell以及什么时候进入子shell](https://www.cnblogs.com/f-ck-need-u/p/7446194.html)

然后再5s后ctx到期cancel后我们再次查看进程树
```golang
systemd(1)─┬─...
           ├─sleep(2684)
           ├─sshd(17395)─┬─sshd(18450)───sftp-server(18453)
           │             ├─sshd(28684)───zsh(28687)───go(2661)─┬─demo3_cmdPit(2680)─┬─{demo3_cmdPit}(2681)
           │             │                                     │                    ├─{demo3_cmdPit}(2682)
           │             │                                     │                    └─{demo3_cmdPit}(2686)
           │             │                                     ├─{go}(2662)
           │             │                                     ├─{go}(2663)
           │             │                                     ├─{go}(2664)
           │             │                                     ├─{go}(2665)
           │             │                                     ├─{go}(2672)
           │             │                                     ├─{go}(2679)
           │             │                                     └─{go}(2685)
           │             └─sshd(29042)───zsh(29044)───pstree(2708)
```
可以看到`bash(2683)`进程确实被kill了，但是它的子进程确并没有结束，而是变成了**孤儿进程**，被`init 1(systemd)`收养，但是我们看到这时进程`demo3_cmdPit(2680)`还没有退出，符合之前的预测，这里该进程仍然还有3条线程（协程）没有退出，直到sleep命令执行完才会返回，这里通过pstack是无法查看这几条线程的堆栈的，因为操作系统并不认识协程，而且G会在P上切换，所以我们暂时也不知道这几条线程在干嘛

>孤儿进程：父进程退出，而它的一个或多个子进程还在运行，那么那些子进程将成为孤儿进程。孤儿进程将被init进程(进程号为1)所收养，并由init进程对它们完成状态收集工作。
>
>僵尸进程：父进程使用fork创建子进程，如果子进程退出，而父进程并没有调用`wait`或`waitpid`获取子进程的状态信息，那么子进程的进程描述符仍然保存在系统中。这种进程称之为僵死进程。

当sleep执行完成后，程序会正常返回如下信息
```go
output：【】err:【signal: killed】#
```
因为是在Linux环境下没有IDE而且不太熟悉`gdb`所以这里我们用`pprof`查看了一下堆栈
```golang
import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os/exec"
	"time"
)

func main() {
	go func() {
		err := http.ListenAndServe(":6060", nil)
		if err != nil {
			fmt.Printf("failed to start pprof monitor:%s", err)
		}
	}()
	ctx, cancelFn := context.WithTimeout(context.Background(), time.Second*5)
	defer cancelFn()
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", "sleep 50; echo hello")
	output, err := cmd.CombinedOutput()
	fmt.Printf("output：【%s】err:【%s】", string(output), err)
}
```
> 后面了解到其实直接发送`SIGQUIT`信号也可以看到堆栈，如 `kill -SIGQUIT <pid>`，go的工具链还是非常完善的，除了这些还有很多方法看堆栈


```go
curl http://127.0.0.1:6060/debug/pprof/goroutine\?debug\=2
...
goroutine 1 [chan receive]:
os/exec.(*Cmd).Wait(0xc000098580, 0x0, 0x0)
        /usr/lib/golang/src/os/exec/exec.go:509 +0x125
os/exec.(*Cmd).Run(0xc000098580, 0xc00006d710, 0xc000098580)
        /usr/lib/golang/src/os/exec/exec.go:341 +0x5c
os/exec.(*Cmd).CombinedOutput(0xc000098580, 0x9, 0xc0000bff30, 0x2, 0x2, 0xc000098580)
        /usr/lib/golang/src/os/exec/exec.go:561 +0x91
main.main()
		/usr/local/gotest/cmd/demo3_cmdPit/main.go:22 +0x176
...
goroutine 9 [IO wait]:
internal/poll.runtime_pollWait(0x7fa96a0f5f08, 0x72, 0xffffffffffffffff)
        /usr/lib/golang/src/runtime/netpoll.go:184 +0x55
internal/poll.(*pollDesc).wait(0xc000058678, 0x72, 0x201, 0x200, 0xffffffffffffffff)
        /usr/lib/golang/src/internal/poll/fd_poll_runtime.go:87 +0x45
internal/poll.(*pollDesc).waitRead(...)
        /usr/lib/golang/src/internal/poll/fd_poll_runtime.go:92
internal/poll.(*FD).Read(0xc000058660, 0xc0000e2000, 0x200, 0x200, 0x0, 0x0, 0x0)
        /usr/lib/golang/src/internal/poll/fd_unix.go:169 +0x1cf
os.(*File).read(...)
        /usr/lib/golang/src/os/file_unix.go:259
os.(*File).Read(0xc000010088, 0xc0000e2000, 0x200, 0x200, 0x7fa96a0f5ff8, 0x0, 0xc000039ea0)
        /usr/lib/golang/src/os/file.go:116 +0x71
bytes.(*Buffer).ReadFrom(0xc00006d710, 0x8b6120, 0xc000010088, 0x7fa96a0f5ff8, 0xc00006d710, 0x1)
        /usr/lib/golang/src/bytes/buffer.go:204 +0xb4
io.copyBuffer(0x8b5a20, 0xc00006d710, 0x8b6120, 0xc000010088, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0)
        /usr/lib/golang/src/io/io.go:388 +0x2ed
io.Copy(...)
        /usr/lib/golang/src/io/io.go:364
os/exec.(*Cmd).writerDescriptor.func1(0x0, 0x0)
        /usr/lib/golang/src/os/exec/exec.go:311 +0x63
os/exec.(*Cmd).Start.func1(0xc000098580, 0xc00000e3a0)
        /usr/lib/golang/src/os/exec/exec.go:435 +0x27
created by os/exec.(*Cmd).Start
        /usr/lib/golang/src/os/exec/exec.go:434 +0x608
```
这里我忽略了一些无关的协程，留下了这两条的堆栈信息

首先我们看一下第一条的堆栈信息，很明显这条就是我们的主`goroutine`，可以看到它正处于`[chan receive]`状态，也就是在等待某个chan传递信息，根据堆栈信息可以看到问题出在

 `/usr/lib/golang/src/os/exec/exec.go:509`

我们去看一下这里发生了什么

![mark](http://static.imlgw.top/blog/20200821/l2wEHHBXyTTh.png?imageslim)
果然这里在等待`c.errch`，那么这个errch是谁负责推送的呢？

我们再来看看另一条`goroutine`，这条`goroutine`处于`【IO wait】`状态，说明IO发生阻塞了，我们从下往上看堆栈，首先看一下是在哪里启动了这个协程
```go
created by os/exec.(*Cmd).Start
        /usr/lib/golang/src/os/exec/exec.go:434 +0x608
```
![mark](http://static.imlgw.top/blog/20200822/sh8qmlcDeD64.png?imageslim)
可以看到这个协程的作用就是向我们上面的主`goroutine`在等待的`c.errch`推送消息，那么这里`fn()`为什么没有返回呢？这个fn是在干嘛？继续翻一翻源码，发现这些函数`fn()`是在cmd.Start的时候创建的，用于处理shell的标准输入，输出和错误输出，而我们的程序就是卡在了这里，继续跟进上面的堆栈信息
```go
os/exec.(*Cmd).writerDescriptor.func1(0x0, 0x0)
        /usr/lib/golang/src/os/exec/exec.go:311 +0x63
```
![UTOOLS1598032678130.png](https://upload.cc/i1/2020/08/22/ls251C.png)
可以看到这里就是具体协程执行的任务，这里有一个IO操作，可想而知程序就是阻塞在了这里，再往下分析就是内部epoll的代码了，就不往下了，我们分析下这段代码的意义，首先创建了管道`Pipe`，然后在协程中将读端`pr`的数据copy到`w`中，很明显这里就是`goroutine`通过pipe读取子进程的输出，但是由于某些原因`Pipe`阻塞了，无法从读端获取数据

我们用`lsof -p`看一下该进程打开的资源
```go
$ lsof -p 27301
COMMAND   PID USER   FD      TYPE    DEVICE SIZE/OFF      NODE NAME
main    27301 root  cwd       DIR     253,1     4096   1323417 /usr/local/gotest/cmd/demo3_cmdPit
main    27301 root  rtd       DIR     253,1     4096         2 /
main    27301 root  txt       REG     253,1  7647232    401334 /tmp/go-build043247660/b001/exe/main
main    27301 root  mem       REG     253,1  2151672   1050229 /usr/lib64/libc-2.17.so
main    27301 root  mem       REG     253,1   141968   1050255 /usr/lib64/libpthread-2.17.so
main    27301 root  mem       REG     253,1   163400   1050214 /usr/lib64/ld-2.17.so
main    27301 root    0u      CHR     136,0      0t0         3 /dev/pts/0
main    27301 root    1u      CHR     136,0      0t0         3 /dev/pts/0
main    27301 root    2u      CHR     136,0      0t0         3 /dev/pts/0
main    27301 root    3u     IPv6 170355813      0t0       TCP *:6060 (LISTEN)
main    27301 root    4u  a_inode      0,10        0      5074 [eventpoll]
main    27301 root    5r     FIFO       0,9      0t0 170355794 pipe
```
可以看到确实打开了一个inode为170355794的管道，结合前面的分析，我们的shell在执行的时候`fork()`创建了子进程，但是kill的时候只kill了shell进程，而在`fork()`的时候会同时将当前进程的`pipe`的`fd[2]`描述符也复制过去，这就造成了`pipe`的写入端`fd[1]`被子进程继续持有（文件表引用计数不为0），而`goroutine`也会继续等待pipe写入端写入或者关闭，直到`sleep 50`执行完后才返回
### 原因总结
`golang`的`os/exec`包在和shell做交互的时候会和shell之间建立pipe，用于输入输出，获取shell返回值，但是在某些情况下，我们的shell会fork出子进程去执行命令，比如多条语句，&后台执行，sh脚本等，然而这里ctx过期后，仅仅kill了shell进程，并没有kill它所创建的子进程，这就导致了pipe的fd仍然被子进程持有无法关闭，而在`os/exec`的实现中`goroutine`会一直等待pipe关闭后才返回，进而导致了该问题的产生
### 解决方案
问题搞清楚了，就好解决了。在go的实现中只kill了shell进程，并不会kill子进程，进而引发了上面的问题
```golang
// Kill causes the Process to exit immediately. Kill does not wait until
// the Process has actually exited. This only kills the Process itself,
// not any other processes it may have started.
func (p *Process) Kill() error {
	return p.kill()
}
```
这里我们可以通过kill`进程组`的方式来解决，所谓进程组也就是父进程和其创建的子进程的集合，`PGID`就是进程组的ID，每个进程组都有进程组ID，这里我们可以通过`ps -axjf`命令查看一下
```go
PPID   PID  PGID   SID TTY      TPGID STAT   UID   TIME COMMAND
17395 20937 20937 20937 ?           -1 Ss       0   0:00  \_ sshd: root@pts/0
20937 20939 20939 20939 pts/0    25879 Ss       0   0:00  |   \_ -zsh
20939 25879 25879 20939 pts/0    25879 Sl+      0   0:00  |       \_ go run main.go
25879 25902 25879 20939 pts/0    25879 Sl+      0   0:00  |           \_ /tmp/go-build486839449/b001/exe/main
25902 25906 25879 20939 pts/0    25879 S+       0   0:00  |               \_ /bin/bash -c sleep 50; echo hello
25906 25907 25879 20939 pts/0    25879 S+       0   0:00  |                   \_ sleep 50
```
在知道`PGID`后可以根据这个ID直接Kill掉所有相关的进程，但是这里注意看上面，我们的`Shell`进程和它的子进程，以及我们`go进程`都是在一个进程组内的，直接kill会把我们的go进程也杀死，所以这里我们要让`Shell`进程和它的子进程额外开辟一个新的进程组，然后再kill

> 下面的代码中使用到了一些`syscall`包的方法，这个包的实现平台之间是有差异的，下面的代码只适用于Linux环境下

```golang
func RunCmd(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var (
		b   bytes.Buffer
		err error
	)
	//开辟新的线程组（Linux平台特有的属性）
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, //使得Shell进程开辟新的PGID,即Shell进程的PID,它后面创建的所有子进程都属于该进程组
	}
	cmd.Stdout = &b
	cmd.Stderr = &b
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	var finish = make(chan struct{})
	defer close(finish)
	go func() {
		select {
		case <-ctx.Done(): //超时/被cancel 结束
			//kill -(-PGID)杀死整个进程组
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-finish: //正常结束
		}
	}()
	//wait等待goroutine执行完，然后释放FD资源
	//这个时候再kill掉shell进程就不会再等待了，会直接返回
	if err = cmd.Wait(); err != nil {
		return nil, err
	}
	return b.Bytes(), err
}
```
**效果演示**
![mark](http://static.imlgw.top/blog/20200825/gfz2xQ7nnKKg.gif)