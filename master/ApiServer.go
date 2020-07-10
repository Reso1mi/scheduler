package master

import (
	"encoding/json"
	"github.com/imlgw/scheduler/master/common"
	"net"
	"net/http"
	"strconv"
	"time"
)

var (
	G_apiServer *ApiServer
)

//任务的Http接口
type ApiServer struct {
	httpServer *http.Server
}

func InitApiServer() error {
	var (
		mux        *http.ServeMux
		listener   net.Listener
		httpServer *http.Server
		err        error
	)
	mux = http.NewServeMux()
	mux.HandleFunc("/job/save", handleJobSave)
	//启动TCP监听(更底层的操作)
	if listener, err = net.Listen("tcp", ":"+strconv.Itoa(G_config.ApiPort)); err != nil {
		return err
	}
	//创建一个Http服务
	httpServer = &http.Server{
		ReadTimeout:  time.Duration(G_config.ApiReadTimeout) * time.Millisecond,
		WriteTimeout: time.Duration(G_config.ApiWriteTimeout) * time.Millisecond,
		Handler:      mux,
	}
	G_apiServer = &ApiServer{
		httpServer: httpServer,
	}
	//启动了服务端
	go httpServer.Serve(listener)
	return err
}

//保存任务接口
//POST job={"name":"job1", "command": "echo hello", "cronExpr":"*****"}
func handleJobSave(w http.ResponseWriter, req *http.Request) {
	var (
		err       error
		postJob   string
		job       common.Job
		oldJob    common.Job
		respBytes []byte
	)
	//获取表单中的job
	postJob = req.PostFormValue("job")
	//反序列化(这里踩了个小坑，明天记录下)
	//todo 测试下Unmarshal
	if err = json.Unmarshal([]byte(postJob), &job); err != nil {
		goto ERR
	}
	//保存到etcd
	if oldJob, err = G_jobManager.SaveJob(job); err != nil {
		goto ERR
	}
	//响应客户端（{"error"}）
	if respBytes, err = common.BuildResp(0, "success", oldJob); err != nil {
		goto ERR
	}
	w.Write(respBytes)
	return //感觉用了goto之后需要额外的注意return...
ERR:
	//异常响应
	if respBytes, err = common.BuildResp(1, err.Error(), nil); err != nil {
		w.Write(respBytes)
	}
}
