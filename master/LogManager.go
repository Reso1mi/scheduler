package master

import (
	"fmt"
	"github.com/imlgw/scheduler/common"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type LogManager struct {
	mysqlDB *gorm.DB
}

//noinspection all
var (
	G_logManager *LogManager
)

//Master做查询日志的管理器
func InitLogManager() error {
	var (
		err error
		DB  *gorm.DB
	)
	conn := fmt.Sprintf("%s:%s@tcp(%s)/%s", G_config.DbUsername, G_config.DbPassword, G_config.DbUri, G_config.DbName)
	if DB, err = gorm.Open(mysql.Open(conn), &gorm.Config{
		//取代之前的 MysqlDB.SingularTable(true)
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	}); err != nil {
		panic("数据库连接失败：" + err.Error())
	}
	G_logManager = &LogManager{
		mysqlDB: DB,
	}
	return err
}

//查询该任务的日志
func (logManager *LogManager) ListLogs(name string, offset int, limit int) ([]*common.JobLog, error) {
	var (
		err  error
		logs []*common.JobLog
	)
	if err = logManager.mysqlDB.Where("job_name = ?", name).Offset(offset).Limit(limit).Order("start_time desc").Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, err
}
