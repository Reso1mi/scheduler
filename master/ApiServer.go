package master

import (
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
func handleJobSave(w http.ResponseWriter, r *http.Request) {

}
