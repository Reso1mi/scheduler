package worker

import (
	"fmt"
	"github.com/imlgw/scheduler/common"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"time"
)

type LogSink struct {
	mysqlDB        *gorm.DB
	logChan        chan *common.JobLog
	autoCommitChan chan *common.LogBatch
}

//noinspection ALL
var (
	G_logSink *LogSink
)

func InitLogSink() error {
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
	//建表
	DB.AutoMigrate(&common.JobLog{})
	G_logSink = &LogSink{
		mysqlDB:        DB,
		logChan:        make(chan *common.JobLog, G_config.LogChanLen),
		autoCommitChan: make(chan *common.LogBatch, G_config.LogChanLen),
	}
	//启动一个db处理协程
	go G_logSink.writeLog()
	return err
}

//写日志，循环读取chan中的log
func (logSink *LogSink) writeLog() {
	var (
		log         *common.JobLog
		logBatch    *common.LogBatch //当前日志批次
		commitTimer *time.Timer
	)
	for {
		select {
		case log = <-logSink.logChan:
			//将log写入db
			if logBatch == nil {
				logBatch = &common.LogBatch{}
				//让这个批次超时自动提交
				commitTimer = time.AfterFunc(
					time.Duration(G_config.JobLogCommitTimeOut)*time.Millisecond,
					func(batch *common.LogBatch) func() {
						//发出超时通知，将batch推送到chan中不直接提交batch，避免并发操作batch
						//闭包，将batch指针私有化（外部仍然可以继续append值），避免后面清空被赋值为nil，投递nil消息
						return func() {
							logSink.autoCommitChan <- batch
						}
					}(logBatch),
				)
			}
			logBatch.Logs = append(logBatch.Logs, log)
			if len(logBatch.Logs) >= G_config.JobLogBatchSize {
				//发送日志到DB
				logSink.saveLogs(logBatch)
				//清空batch
				logBatch = nil
				//取消定时器，但是有可能定时器执行完成了，超时通知已经发送了，所以我们下面需要额外的判断一下
				commitTimer.Stop()
			}
		case timeOutBatch := <-logSink.autoCommitChan:
			//判断还是不是当前的批次，如果不是就说明之前的已经提交了
			if timeOutBatch != logBatch {
				continue
			}
			//处理过期的batch
			logSink.saveLogs(timeOutBatch)
			//清空batch
			logBatch = nil
		}
	}
}

//批量写入日志
func (logSink *LogSink) saveLogs(batch *common.LogBatch) {
	logSink.mysqlDB.Create(batch.Logs)
}

func (logSink *LogSink) AppendLog(jobLog *common.JobLog) {
	select {
	case logSink.logChan <- jobLog: //可写状态
	default:
		//不可写状态，也就是chan满了，队列满之后暂时先直接丢弃
	}
}
