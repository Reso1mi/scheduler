package worker

import (
	"github.com/imlgw/scheduler/common"
	"os/exec"
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
			output, err = cmd.CombinedOutput()
			//记录任务结束时间
			result.EndTime = time.Now()
			result.Output = output
			result.Err = err
		}
		//任务执行完成后，把执行的结果返回给Scheduler，Scheduler会从executeTable删除任务
		G_scheduler.PushJobResult(result)
	}()
}

//初始化执行器
func InitExecutor() error {
	return nil
}
