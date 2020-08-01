package master

import (
	"encoding/json"
	"github.com/imlgw/scheduler/common"
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
		mux           *http.ServeMux
		listener      net.Listener
		httpServer    *http.Server
		err           error
		staticDir     http.Dir
		staticHandler http.Handler
	)
	mux = http.NewServeMux()
	mux.HandleFunc("/job/save", handleJobSave)
	mux.HandleFunc("/job/delete", handleJobDelete)
	mux.HandleFunc("/job/list", handleJobList)
	mux.HandleFunc("/job/kill", handleJobKill)
	mux.HandleFunc("/job/log", handleJobLog)
	//静态文件目录
	staticDir = http.Dir(G_config.Webapp)
	staticHandler = http.FileServer(staticDir)
	mux.Handle("/", staticHandler)
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

//查询任务日志
func handleJobLog(w http.ResponseWriter, req *http.Request) {
	var (
		err       error
		name      string
		offset    int
		limit     int
		respBytes []byte
		logs      []*common.JobLog
	)
	//获取请求参数 /job/log?name=job&offset=0&limit=10
	name = req.FormValue("name")
	if offset, err = strconv.Atoi(req.FormValue("offset")); err != nil {
		offset = 0
	}
	if limit, err = strconv.Atoi(req.FormValue("limit")); err != nil {
		limit = 20
	}
	if logs, err = G_logManager.ListLogs(name, offset, limit); err != nil {
		goto ERR
	}
	//响应客户端
	if respBytes, err = common.BuildResp(0, "success", logs); err != nil {
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

//保存任务接口
//POST job={"name":"job1", "command": "echo hello", "cronExpr":"*****"}
func handleJobSave(w http.ResponseWriter, req *http.Request) {
	var (
		err       error
		postJob   string
		job       *common.Job
		oldJob    *common.Job
		respBytes []byte
	)
	//获取表单中的job
	postJob = req.PostFormValue("job")
	//反序列化(这里踩了个小坑，第二个参数一开始传递的job，相当于传递了一个nil)
	//如果传&job(指针的地址) 实际上这里还是没有赋值，只不过传指针的地址进去后底层会自动的创建一个，但是传一个指针的地址太奇怪了
	job = &common.Job{} //这样比较好
	if err = json.Unmarshal([]byte(postJob), job); err != nil {
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

//删除任务接口
//POST:name="job1"
func handleJobDelete(w http.ResponseWriter, req *http.Request) {
	var (
		err       error
		jobName   string
		oldJob    *common.Job
		respBytes []byte
	)
	if err = req.ParseForm(); err != nil {
		goto ERR
	}
	jobName = req.PostFormValue("name")
	if oldJob, err = G_jobManager.DeleteJob(jobName); err != nil {
		goto ERR
	}
	if respBytes, err = common.BuildResp(0, "success", oldJob); err != nil {
		goto ERR
	}
	w.Write(respBytes)
	return
ERR:
	//异常响应
	if respBytes, err = common.BuildResp(-1, err.Error(), nil); err != nil {
		w.Write(respBytes)
	}
}

func handleJobList(w http.ResponseWriter, req *http.Request) {
	var (
		err       error
		jobList   []*common.Job
		respBytes []byte
	)
	if jobList, err = G_jobManager.ListJob(); err != nil {
		goto ERR
	}
	if respBytes, err = common.BuildResp(0, "success", jobList); err != nil {
		goto ERR
	}
	w.Write(respBytes)
	return
ERR:
	//异常响应
	if respBytes, err = common.BuildResp(-1, err.Error(), nil); err != nil {
		w.Write(respBytes)
	}
}

//强制杀死某个任务
//POST /job/kill name = jobname
func handleJobKill(w http.ResponseWriter, req *http.Request) {
	var (
		err       error
		jobName   string
		respBytes []byte
	)
	jobName = req.PostFormValue("name")
	if err = G_jobManager.KillJob(jobName); err != nil {
		goto ERR
	}
	if respBytes, err = common.BuildResp(0, "success", nil); err != nil {
		goto ERR
	}
	w.Write(respBytes)
	return
ERR:
	//异常响应
	if respBytes, err = common.BuildResp(-1, err.Error(), nil); err != nil {
		w.Write(respBytes)
	}
}
