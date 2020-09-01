package worker

import (
	"context"
	"fmt"
	"github.com/coreos/etcd/clientv3"
	"github.com/imlgw/scheduler/common"
	"net"
	"time"
)

type Register struct {
	client *clientv3.Client
	kv     clientv3.KV
	lease  clientv3.Lease
	//本机IP
	localIP string
}

//noinspection ALL
var (
	G_register *Register
)

func InitRegister() error {
	var (
		config  clientv3.Config
		kv      clientv3.KV
		lease   clientv3.Lease
		client  *clientv3.Client
		localIP string
		err     error
	)
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

	if localIP, err = getLocalIP(); err != nil {
		return err
	}
	G_register = &Register{
		client:  client,
		kv:      kv,
		lease:   lease,
		localIP: localIP,
	}
	go G_register.keepOnline()
	return nil
}

//注册到etcd并且自动续租
func (register *Register) keepOnline() {
	var (
		regKey         string
		err            error
		leaseGrantResp *clientv3.LeaseGrantResponse
		keepRespChan   <-chan *clientv3.LeaseKeepAliveResponse
		canCtx         context.Context
		canFunc        context.CancelFunc
	)
	for {

		regKey = common.JOB_WORKERS_DIR + register.localIP
		if leaseGrantResp, err = register.lease.Grant(context.TODO(), 10); err != nil {
			goto RETRY
		}
		//自动续租，确认机器在线
		if keepRespChan, err = register.lease.KeepAlive(context.TODO(), leaseGrantResp.ID); err != nil {
			goto RETRY
		}
		canCtx, canFunc = context.WithCancel(context.TODO())
		//注册到etcd
		if _, err = register.kv.Put(canCtx, regKey, "", clientv3.WithLease(leaseGrantResp.ID)); err != nil {
			goto RETRY
		}
		//监听续租结果
		for {
			select {
			case keepResp := <-keepRespChan:
				if keepResp == nil { //续租失败
					goto RETRY
				}
			}
		}
	RETRY:
		time.Sleep(1 * time.Second)
		if canFunc != nil { //put失败就取消租约
			canFunc()
		}
	}
}

func getLocalIP() (string, error) {
	var (
		adders []net.Addr
		err    error
	)
	if adders, err = net.InterfaceAddrs(); err != nil {
		return "", err
	}
	//取第一个非回环地址（localhost）的网卡IP
	for _, addr := range adders {
		//ipv4 | ipv6
		//如果能反解成功说明是ip地址
		if ipNet, isIpNet := addr.(*net.IPNet); isIpNet && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.String(), nil //192.168.25.4
			}
		}
	}
	err = common.ERR_NO_LOCAL_IP_FOUND
	return "", err
}
