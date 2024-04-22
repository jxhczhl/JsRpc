package main

import (
	"JsRpc/config"
	"JsRpc/core"
	"JsRpc/utils"
)

func main() {
	utils.PrintJsRpc() // 开屏打印

	baseConf := config.ReadConf()       // 读取日志信息
	utils.InitLogger(baseConf.CloseLog) // 初始化日志
	core.InitAPI(baseConf)              // 初始化api部分

	utils.CloseTerminal() // 安全退出
}
