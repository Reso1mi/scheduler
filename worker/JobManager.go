package worker

import (
	"context"
	"fmt"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/imlgw/scheduler/common"
	"go.etcd.io/etcd/clientv3"
	"time"
)

type JobManager struct {
	client  *clientv3.Client
	kv      clientv3.KV
	lease   clientv3.Lease
	watcher clientv3.Watcher
}

//noinspection ALL
var (
	G_jobManager *JobManager
)

func InitJobManager() error {
	var (
		config  clientv3.Config
		kv      clientv3.KV
		lease   clientv3.Lease
		client  *clientv3.Client
		watcher clientv3.Watcher
		err     error
	)
	//初始化配置
	config = clientv3.Config{
		Endpoints:   G_config.EtcdEndPoints,
		DialTimeout: time.Duration(G_config.EtcdDialTimeOut) * time.Millisecond,
	}
	fmt.Println(config.Endpoints)
	//创建客户端
	if client, err = clientv3.New(config); err != nil {
		return err
	}
	kv = clientv3.NewKV(client)
	lease = clientv3.NewLease(client)
	watcher = clientv3.NewWatcher(client)
	//初始化全局变量
	G_jobManager = &JobManager{
		client:  client,
		kv:      kv,
		lease:   lease,
		watcher: watcher,
	}
	//启动监听
	G_jobManager.watchJobs()
	return err
}

//监听任务变化
func (jobMgr *JobManager) watchJobs() error {
	var (
		err                error
		getResp            *clientv3.GetResponse
		kvPair             *mvccpb.KeyValue
		job                *common.Job
		watchStartRevision int64
		watchChan          clientv3.WatchChan
		watchResp          clientv3.WatchResponse
		watchEvent         *clientv3.Event
		jobName            string
		jobEvent           *common.JobEvent
	)
	//1. get 当前的所有任务 获取revision
	if getResp, err = jobMgr.kv.Get(context.TODO(), common.JOB_SAVE_DIR, clientv3.WithPrefix()); err != nil {
		return err
	}
	for _, kvPair = range getResp.Kvs {
		//反序列化得到job
		if job, err = common.UnmarshalJob(kvPair.Value); err == nil {
			jobEvent = common.BuildJobEvent(common.JOB_EVENT_SAVE, job)
			//将job同步给scheduler(调度协程)
			G_scheduler.PushJobEvent(jobEvent)
		}
	}
	//2. 从该revision向后监听事件变化
	go func() {
		//从GET时刻后序版本开始监听
		watchStartRevision = getResp.Header.Revision + 1
		//监听SAVE_DIR的后续变化
		watchChan = jobMgr.watcher.Watch(context.TODO(), common.JOB_SAVE_DIR, clientv3.WithRev(watchStartRevision), clientv3.WithPrefix())
		for watchResp = range watchChan {
			for _, watchEvent = range watchResp.Events {
				switch watchEvent.Type {
				case mvccpb.PUT: //任务保存事件
					if job, err = common.UnmarshalJob(watchEvent.Kv.Value); err != nil {
						continue
					}
					//构造更新Event
					jobEvent = common.BuildJobEvent(common.JOB_EVENT_SAVE, job)
					//TODO:反序列化，推送给scheduler
				case mvccpb.DELETE: //任务删除事件
					jobName = common.StripDir(string(watchEvent.Kv.Key))
					job = &common.Job{Name: jobName}
					//构造删除Event
					jobEvent = common.BuildJobEvent(common.JOB_EVENT_DELETE, job)
					//TODO: 谁送删除事件scheduler
				}
				//推送给scheduler
				G_scheduler.PushJobEvent(jobEvent)
			}
		}
	}()
	return err
}

//创建任务执行锁
func (jobMgr *JobManager) CreateJobLock(jobName string) *JobLock {
	return InitJobLock(jobName, jobMgr.kv, jobMgr.lease)
}
