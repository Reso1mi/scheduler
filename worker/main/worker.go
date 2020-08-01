package main

import (
	"flag"
	"fmt"
	"github.com/imlgw/scheduler/worker"
	"runtime"
	"time"
)

func initEnv() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

var (
	confFile string
)

//解析命令行参数
func initArgs() {
	//master -config ./master.json
	flag.StringVar(&confFile, "config", "./worker.json", "指定worker.json配置文件")
	flag.Parse()
}

func main() {
	var (
		err error
	)
	//初始化命令行参数
	initArgs()
	//初始化线程
	initEnv()
	//加载配置
	if err = worker.InitConfig(confFile); err != nil {
		goto ERR
	}
	//日志（append）管理器
	if err = worker.InitLogSink(); err != nil {
		goto ERR
	}
	//任务执行器
	if err = worker.InitExecutor(); err != nil {
		goto ERR
	}
	//任务调度器
	if err = worker.InitScheduler(); err != nil {
		goto ERR
	}
	//任务管理器
	if err = worker.InitJobManager(); err != nil {
		goto ERR
	}
	for {
		time.Sleep(time.Second)
	}
	return
ERR:
	fmt.Println(err)
}
