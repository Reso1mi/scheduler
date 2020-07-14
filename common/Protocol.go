package common

import (
	"encoding/json"
)

type Job struct {
	Name     string `json:"name"`     //任务名
	Command  string `json:"command"`  //shell命令
	CronExpr string `json:"cronExpr"` //cron表达式
}

type Response struct {
	ErrorNo int         `json:"errno"`
	Msg     string      `json:"msg"`
	Data    interface{} `json:"data"`
}

func BuildResp(errno int, msg string, data interface{}) ([]byte, error) {
	var (
		err      error
		respObj  *Response
		response []byte
	)
	respObj = &Response{
		ErrorNo: errno,
		Msg:     msg,
		Data:    data,
	}
	if response, err = json.Marshal(respObj); err != nil {
		return nil, err
	}
	return response, err
}
