package worker

import (
	"context"
	"github.com/imlgw/scheduler/common"
	"go.etcd.io/etcd/clientv3"
)

type JobLock struct {
	kv         clientv3.KV
	leases     clientv3.Lease
	jobName    string             //任务命
	cancelFunc context.CancelFunc //用于终止自动续租
	leaseId    clientv3.LeaseID
	isLock     bool
}

func InitJobLock(jobName string, kv clientv3.KV, lease clientv3.Lease) (jobLock *JobLock) {
	return &JobLock{
		kv:      kv,
		leases:  lease,
		jobName: jobName,
	}
}

func (jobLock *JobLock) TryLock() error {
	var (
		err            error
		lockKey        string
		leaseGrantResp *clientv3.LeaseGrantResponse
		cancelCtx      context.Context
		cancelFunc     context.CancelFunc
		leaseId        clientv3.LeaseID
		keepRespChan   <-chan *clientv3.LeaseKeepAliveResponse
		txn            clientv3.Txn
		txnResp        *clientv3.TxnResponse
	)
	//1.创建租约
	if leaseGrantResp, err = jobLock.leases.Grant(context.TODO(), 5); err != nil {
		return err
	}
	cancelCtx, cancelFunc = context.WithCancel(context.TODO())
	leaseId = leaseGrantResp.ID
	//2.自动续租
	if keepRespChan, err = jobLock.leases.KeepAlive(cancelCtx, leaseId); err != nil {
		goto FAIL
	}
	go func() {
		var (
			keepResp *clientv3.LeaseKeepAliveResponse
		)
		for {
			select {
			case keepResp = <-keepRespChan:
				if keepResp == nil {
					goto END
				}
			}
		}
	END:
	}()
	//3.创建事务txn
	txn = jobLock.kv.Txn(context.TODO())
	lockKey = common.JOB_LOCK_DIR + jobLock.jobName
	//4.事务枪锁
	txn.If(clientv3.Compare(clientv3.CreateRevision(lockKey), "=", 0)).
		Then(clientv3.OpPut(lockKey, "{locked}", clientv3.WithLease(leaseId))).
		Else(clientv3.OpGet(lockKey))
	if txnResp, err = txn.Commit(); err != nil { //这里可能写入了，也可能没写入，既然出现了错误就直接回滚
		goto FAIL
	}
	//5.成功返回，失败释放租约
	if !txnResp.Succeeded { //锁被占用，枪锁失败
		err = common.ERR_LOCK_ALREADY_OCCUPY
		goto FAIL
	}
	jobLock.leaseId = leaseId
	jobLock.cancelFunc = cancelFunc
	jobLock.isLock = true
	return err
FAIL:
	cancelFunc()
	jobLock.leases.Revoke(context.TODO(), leaseId)
	return err
}

func (jobLock *JobLock) Unlock() {
	if jobLock.isLock { //上锁成功才释放锁
		jobLock.cancelFunc()                                   //取消自动续租的协程
		jobLock.leases.Revoke(context.TODO(), jobLock.leaseId) //释放租约，相关联的key会自动删除
	}
}
