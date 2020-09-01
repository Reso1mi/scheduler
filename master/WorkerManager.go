package master

import (
	"context"
	"github.com/imlgw/scheduler/common"
	"go.etcd.io/etcd/clientv3"
	"time"
)

type WorkerManager struct {
	client *clientv3.Client
	kv     clientv3.KV
}

//noinspection all
var (
	G_workerManager *WorkerManager
)

func InitWorkerManager() error {
	var (
		config clientv3.Config
		kv     clientv3.KV
		client *clientv3.Client
		err    error
	)
	//初始化配置
	config = clientv3.Config{
		Endpoints:   G_config.EtcdEndPoints,
		DialTimeout: time.Duration(G_config.EtcdDialTimeOut) * time.Millisecond,
	}
	//创建客户端
	if client, err = clientv3.New(config); err != nil {
		return err
	}
	kv = clientv3.NewKV(client)
	//初始化全局变量
	G_workerManager = &WorkerManager{
		client: client,
		kv:     kv,
	}
	return err
}

func (workerMgr *WorkerManager) ListWorkers() ([]string, error) {
	var (
		dirKey  string
		err     error
		getResp *clientv3.GetResponse
	)
	dirKey = common.JOB_WORKERS_DIR
	if getResp, err = workerMgr.kv.Get(context.TODO(), dirKey, clientv3.WithPrefix()); err != nil {
		return nil, err
	}
	workerList := make([]string, 0)
	for _, kvPair := range getResp.Kvs {
		// '/cron/workers/192.168.1.11' 解析成ip地址
		workerList = append(workerList, common.StripDir(common.JOB_WORKERS_DIR, string(kvPair.Key)))
	}
	return workerList, err
}
