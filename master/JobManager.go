package master

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/imlgw/scheduler/master/common"
	"go.etcd.io/etcd/clientv3"
	"time"
)

type JobManager struct {
	client *clientv3.Client
	kv     clientv3.KV
	lease  clientv3.Lease
}

var (
	G_jobManager *JobManager
)

func InitJobManager() error {
	var (
		config clientv3.Config
		kv     clientv3.KV
		lease  clientv3.Lease
		client *clientv3.Client
		err    error
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
	//初始化全局变量
	G_jobManager = &JobManager{
		client: client,
		kv:     kv,
		lease:  lease,
	}
	return err
}

func (jobMgr *JobManager) SaveJob(job common.Job) (common.Job, error) {
	var (
		err      error
		oldJob   common.Job
		jobKey   string
		jobValue []byte
		putResp  *clientv3.PutResponse
	)
	jobKey = "/cron/jobs/" + job.Name
	if jobValue, err = json.Marshal(job); err != nil {
		return oldJob, err
	}
	//保存到etcd
	if putResp, err = jobMgr.kv.Put(context.TODO(), jobKey, string(jobValue), clientv3.WithPrevKV()); err != nil {
		return oldJob, err
	}
	//获取旧值
	if putResp.PrevKv != nil {
		//反序列化oldKey（忽略错误）
		if err = json.Unmarshal(putResp.PrevKv.Value, &oldJob); err != nil {
			return oldJob, nil
		}
	}
	return oldJob, err
}
