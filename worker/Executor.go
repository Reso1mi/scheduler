package worker

import (
	"bytes"
	"context"
	"github.com/imlgw/scheduler/common"
	"math/rand"
	"os/exec"
	"syscall"
	"time"
)

type Executor struct {
}

//noinspection ALL
var (
	G_executor *Executor
)

//执行一个任务
func (executor *Executor) ExecutorJob(info *common.JobExecuteInfo) {
	go func() {
		var (
			cmd     *exec.Cmd
			err     error
			output  []byte
			result  *common.JobExecuteResult
			jobLock *JobLock
		)
		result = &common.JobExecuteResult{
			ExecuteInfo: info,
			Output:      make([]byte, 0),
		}
		//初始化分布式锁
		jobLock = G_jobManager.CreateJobLock(info.Job.Name)
		//任务开始时间
		result.StartTime = time.Now()
		//尝试上锁
		//TODO: 分布式锁的偏向
		time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
		err = jobLock.TryLock()
		defer jobLock.Unlock()
		if err != nil { //上锁失败
			result.Err = err
			result.EndTime = time.Now()
		} else {
			//真实的启动时间
			result.StartTime = time.Now()
			//创建shell命令，传入cancelCtx
			cmd = exec.CommandContext(info.CancelCtx, G_config.BashPath, "-c", info.Job.Command)
			//执行并捕获输出
			//fix: 强杀任务不正常返回 参考：http://imlgw.top/2020/08/25/golang-cai-keng-exec-qu-xiao-bu-tui-chu/
			output, err = RunCmd(info.CancelCtx, cmd)
			//记录任务结束时间
			result.EndTime = time.Now()
			result.Output = output
			result.Err = err
		}
		//任务执行完成后，把执行的结果返回给Scheduler，Scheduler会从executeTable删除任务
		G_scheduler.PushJobResult(result)
	}()
}

//fix: 强杀任务不正常返回
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

//初始化执行器
func InitExecutor() error {
	return nil
}
