package common

import "errors"

//noinspection ALL
var (
	ERR_LOCK_ALREADY_OCCUPY = errors.New("锁已经被占用了！")
	ERR_NO_LOCAL_IP_FOUND   = errors.New("没有找到网卡IP")
)
