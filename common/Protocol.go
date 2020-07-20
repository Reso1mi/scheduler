package common

import (
	"encoding/json"
	"github.com/gorhill/cronexpr"
	"strings"
	"time"
)

//定时任务
type Job struct {
	Name     string `json:"name"`     //任务名
	Command  string `json:"command"`  //shell命令
	CronExpr string `json:"cronExpr"` //cron表达式
}

//任务调度计划
type JobSchedulerPlan struct {
	Job      *Job                 //要调度的任务信息
	Expr     *cronexpr.Expression //解析过的cronEXpr表达式
	NextTime time.Time            //下次调度的时间
}

//任务执行状态
type JobExecuteInfo struct {
	Job      *Job
	PlanTime time.Time //理论上调度时间
	RealTime time.Time //实际的调度时间
}

//HTTP接口的应答消息
type Response struct {
	ErrorNo int         `json:"errno"`
	Msg     string      `json:"msg"`
	Data    interface{} `json:"data"`
}

//变化事件
type JobEvent struct {
	EventType int
	Job       *Job
}

//任务执行结果
type JobExecuteResult struct {
	ExecuteInfo *JobExecuteInfo //执行状态
	Output      []byte          //shell的输出
	Err         error           //脚本错误原因
	StartTime   time.Time       //启动时间
	EndTime     time.Time       //结束时间
}

func BuildResp(errno int, msg string, data interface{}) ([]byte, error) {
	var (
		err      error
		respObj  *Response
		response []byte
	)
	respObj = &Response{
		ErrorNo: errno,
		Msg:     msg,
		Data:    data,
	}
	if response, err = json.Marshal(respObj); err != nil {
		return nil, err
	}
	return response, err
}

func UnmarshalJob(data []byte) (*Job, error) {
	var job = &Job{}
	if err := json.Unmarshal(data, job); err != nil {
		return nil, err
	}
	return job, nil
}

//删除任务目录，获取任务名
func StripDir(jobKey string) string {
	return strings.TrimPrefix(jobKey, JOB_SAVE_DIR)
}

//构建Event 1) 更新任务 2)删除任务
func BuildJobEvent(eventType int, job *Job) *JobEvent {
	return &JobEvent{
		EventType: eventType,
		Job:       job,
	}
}

func BuildJobSchedulePlan(job *Job) (*JobSchedulerPlan, error) {
	var (
		err  error
		expr *cronexpr.Expression
	)
	if expr, err = cronexpr.Parse(job.CronExpr); err != nil {
		return nil, err
	}
	return &JobSchedulerPlan{
		Job:      job,
		Expr:     expr,
		NextTime: expr.Next(time.Now()),
	}, err
}

func BuildJobExecuteInfo(plan *JobSchedulerPlan) *JobExecuteInfo {
	return &JobExecuteInfo{
		Job:      plan.Job,
		PlanTime: plan.NextTime, //计算调度时间
		RealTime: time.Now(),    //真实执行时间
	}
}
