package worker

import (
	"context"
	"github.com/imlgw/scheduler/common"
	"os/exec"
	"time"
)

type Executor struct {
}

var (
	G_executor *Executor
)

//执行一个任务
func (executor *Executor) ExecutorJob(info *common.JobExecuteInfo) {
	go func() {
		var (
			cmd    *exec.Cmd
			err    error
			output []byte
			result *common.JobExecuteResult
		)
		result = &common.JobExecuteResult{
			ExecuteInfo: info,
			Output:      make([]byte, 0),
		}
		//任务开始时间
		result.StartTime = time.Now()
		//创建shell命令
		cmd = exec.CommandContext(context.TODO(), G_config.BashPath, "-c", info.Job.Command)
		//执行并捕获输出
		output, err = cmd.CombinedOutput()
		//记录任务结束时间
		result.EndTime = time.Now()
		result.Output = output
		result.Err = err
		//任务执行完成后，把执行的结果返回给Scheduler，Scheduler会从executeTable删除任务
		G_scheduler.PushJobResult(result)
	}()
}

//初始化执行器
func InitExecutor() error {
	return nil
}