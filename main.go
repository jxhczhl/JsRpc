package main

import (
	"JsRpc/config"
	"JsRpc/core"
	"JsRpc/utils"
)

func main() {
	utils.PrintJsRpc()                // 开屏打印
	utils.InitLogger()                // 初始化日志
	baseConf := config.ReadConf()     // 读取日志信息
	utils.PrintSet(baseConf.CloseLog) // 关闭部分日志
	core.InitAPI(baseConf)            // 初始化api部分

	utils.CloseTerminal() // 安全退出
}
