package main

import (
	"flag"
	"fmt"
	"github.com/imlgw/scheduler/master"
	"runtime"
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
	flag.StringVar(&confFile, "config", "./master.json", "指定master.json配置文件")
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
	if err = master.InitConfig(confFile); err != nil {
		goto ERR
	}
	//启动APIServer
	if err = master.InitApiServer(); err != nil {
		goto ERR
	}
	return
ERR:
	fmt.Println(err)
}
