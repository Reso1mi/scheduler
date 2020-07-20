package worker

import (
	"fmt"
	"github.com/imlgw/scheduler/common"
	"time"
)

type Scheduler struct {
	jobEventChan      chan *common.JobEvent               //etcd任务事件队列
	jobPlanTable      map[string]*common.JobSchedulerPlan //任务计划表
	jobExecutingTable map[string]*common.JobExecuteInfo
	jobResultChan     chan *common.JobExecuteResult //任务结果队列
}

var (
	G_scheduler *Scheduler
)

//核心调度逻辑
func (scheduler *Scheduler) doScheduler() {
	var (
		jobEvent       *common.JobEvent
		schedulerAfter time.Duration
		schedulerTimer *time.Timer
		jobResult      *common.JobExecuteResult
	)
	//初始化（1s）
	schedulerAfter = scheduler.TrySchedule()
	//调度定时器
	schedulerTimer = time.NewTimer(schedulerAfter)
	//定时任务
	for {
		select {
		case jobEvent = <-scheduler.jobEventChan: //监听任务变化事件
			scheduler.handleEvent(jobEvent)
		case <-schedulerTimer.C: //最近的任务到期了
		case jobResult = <-scheduler.jobResultChan: //监听任务执行结果
			scheduler.handleJobResult(jobResult)
		}
		schedulerAfter = scheduler.TrySchedule()
		//重置定时器
		schedulerTimer.Reset(schedulerAfter)
	}
}

//处理执行完成的任务
func (scheduler *Scheduler) handleJobResult(jobResult *common.JobExecuteResult) {
	//删除执行表中的任务
	delete(scheduler.jobExecutingTable, jobResult.ExecuteInfo.Job.Name)
	fmt.Printf("任务【%s】执行完成, output【%s】,Err【%s】\n", jobResult.ExecuteInfo.Job.Name, string(jobResult.Output), jobResult.Err)
}

//处理任务事件，实时同步etcd中的任务和执行计划表
func (scheduler *Scheduler) handleEvent(jobEvent *common.JobEvent) {
	var (
		err      error
		jobExist bool
		jobPlan  *common.JobSchedulerPlan
	)
	switch jobEvent.EventType {
	case common.JOB_EVENT_SAVE: //保存任务事件：添加进入计划表
		if jobPlan, err = common.BuildJobSchedulePlan(jobEvent.Job); err != nil {
			return
		}
		scheduler.jobPlanTable[jobEvent.Job.Name] = jobPlan
	case common.JOB_EVENT_DELETE: //删除任务事件: 从计划表中删除
		if jobPlan, jobExist = scheduler.jobPlanTable[jobEvent.Job.Name]; jobExist {
			delete(scheduler.jobPlanTable, jobEvent.Job.Name)
		}
	}
}

//尝试调度任务计划表中的任务，并统计最近的一次到期的任务，便于后面精确的睡眠
func (scheduler *Scheduler) TrySchedule() time.Duration {
	var (
		jobSchedulePlan *common.JobSchedulerPlan
		now             time.Time
		nearTime        *time.Time
	)
	if len(scheduler.jobPlanTable) == 0 {
		//没有任务
		return time.Second
	}
	now = time.Now()
	//1. 遍历所有任务
	for _, jobSchedulePlan = range scheduler.jobPlanTable {
		if jobSchedulePlan.NextTime.Before(now) || jobSchedulePlan.NextTime.Equal(now) {
			//2. 过期的任务立即执行
			//尝试执行任务
			scheduler.TryStartJob(jobSchedulePlan)
			fmt.Printf("任务 [%s] 被执行\n", jobSchedulePlan.Job.Name)
			jobSchedulePlan.NextTime = jobSchedulePlan.Expr.Next(now) //更新下次执行时间
		}
		//统计最近一个要过期的任务
		if nearTime == nil || jobSchedulePlan.NextTime.Before(*nearTime) {
			nearTime = &jobSchedulePlan.NextTime
		}
	}
	//3. 统计最近要过期的任务的时间 N秒后过期
	return nearTime.Sub(now)
}

func (scheduler *Scheduler) TryStartJob(jobSchedulerPlan *common.JobSchedulerPlan) {
	//被调度的的任务单次执行时间可能会执行很久，比如1s执行一次，但是一次要执行一分钟
	//所以我们应该去重防止并发
	var (
		jobExecuteInfo *common.JobExecuteInfo
		running        bool
	)
	if jobExecuteInfo, running = scheduler.jobExecutingTable[jobSchedulerPlan.Job.Name]; running {
		//任务正在执行
		fmt.Printf("【%s】任务尚未退出，跳过执行\n", jobSchedulerPlan.Job.Name)
		return
	}
	//构建执行状态
	jobExecuteInfo = common.BuildJobExecuteInfo(jobSchedulerPlan)
	//更新执行表
	scheduler.jobExecutingTable[jobSchedulerPlan.Job.Name] = jobExecuteInfo
	//TODO:启动shell命令
	fmt.Printf("执行【%s】任务\n", jobSchedulerPlan.Job.Name)
	G_executor.ExecutorJob(jobExecuteInfo)
}

//chan推送任务变化事件
func (scheduler *Scheduler) PushJobEvent(jobEvent *common.JobEvent) {
	scheduler.jobEventChan <- jobEvent
}

//chan回传任务执行结果
func (scheduler *Scheduler) PushJobResult(jobResult *common.JobExecuteResult) {
	scheduler.jobResultChan <- jobResult
}

func InitScheduler() error {
	var (
		err error
	)
	G_scheduler = &Scheduler{
		jobEventChan:      make(chan *common.JobEvent, 1000),
		jobPlanTable:      make(map[string]*common.JobSchedulerPlan),
		jobExecutingTable: make(map[string]*common.JobExecuteInfo),
		jobResultChan:     make(chan *common.JobExecuteResult, 1000),
	}
	//启动调度协程
	go G_scheduler.doScheduler()
	return err
}
