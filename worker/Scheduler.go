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
	)
	//初始化（1s）
	schedulerAfter = scheduler.TrySchedule()
	//调度定时器
	schedulerTimer = time.NewTimer(schedulerAfter)
	//定时任务
	for {
		select {
		case jobEvent = <-scheduler.jobEventChan:
			scheduler.handleEvent(jobEvent)
		case <-schedulerTimer.C: //最近的任务到期了
		}
		schedulerAfter = scheduler.TrySchedule()
		//重置定时器
		schedulerTimer.Reset(schedulerAfter)
	}
}

//处理任务事件，实时同步etcd中的任务和执行计划表
func (scheduler *Scheduler) handleEvent(jobEvent *common.JobEvent) {
	var (
		err          error
		jobExist     bool
		jobScheduler *common.JobSchedulerPlan
	)
	switch jobEvent.EventType {
	case common.JOB_EVENT_SAVE: //保存任务事件：添加进入计划表
		if jobScheduler, err = common.BuildJobSchedulePlan(jobEvent.Job); err != nil {
			return
		}
		scheduler.jobPlanTable[jobEvent.Job.Name] = jobScheduler
	case common.JOB_EVENT_DELETE: //删除任务事件: 从计划表中删除
		if jobScheduler, jobExist = scheduler.jobPlanTable[jobEvent.Job.Name]; jobExist {
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
}

//推送任务变化事件
func (scheduler *Scheduler) PushJobEvent(jobEvent *common.JobEvent) {
	scheduler.jobEventChan <- jobEvent
}

func InitScheduler() error {
	var (
		err error
	)
	G_scheduler = &Scheduler{
		jobEventChan:      make(chan *common.JobEvent, 1000),
		jobPlanTable:      make(map[string]*common.JobSchedulerPlan),
		jobExecutingTable: make(map[string]*common.JobExecuteInfo),
	}
	//启动调度协程
	go G_scheduler.doScheduler()
	return err
}
