package master

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	ApiPort         int      `json:"apiPort"`
	ApiReadTimeout  int      `json:"apiReadTimeout"`
	ApiWriteTimeout int      `json:"apiWriteTimeout"`
	EtcdEndPoints   []string `json:"etcdEndpoints"`
	EtcdDialTimeOut int      `json:"etcdDialTimeout"`
	Webapp          string   `json:"webapp"`
	DbUri           string   `json:"dbUri"`
	DbName          string   `json:"dbName"`
	DbUsername      string   `json:"dbUsername"`
	DbPassword      string   `json:"dbPassword"`
}

var (
	G_config Config
)

func InitConfig(filename string) error {
	var (
		content []byte
		conf    Config
		err     error
	)
	if content, err = ioutil.ReadFile(filename); err != nil {
		return err
	}
	//解析配置
	if err = json.Unmarshal(content, &conf); err != nil {
		return err
	}
	G_config = conf
	return err
}
