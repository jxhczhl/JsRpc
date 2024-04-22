package main

import (
	"JsRpc/config"
	"JsRpc/core"
	"JsRpc/utils"
	"flag"
	log "github.com/sirupsen/logrus"
)

func readConf() config.ConfStruct {
	var ConfigPath string
	// 定义命令行参数-c，后面跟着的是默认值以及参数说明
	flag.StringVar(&ConfigPath, "c", "config.yaml", "指定配置文件的路径")
	// 解析命令行参数
	flag.Parse()

	conf, err := config.InitConf(ConfigPath)
	if err != nil {
		log.Errorln("读取配置文件错误，将使用默认配置运行。 ", err.Error())
	}
	return conf
}

func main() {
	utils.PrintJsRpc() // 开屏打印
	baseConf := readConf()

	utils.InitLogger(baseConf.CloseLog) // 初始化日志
	core.InitAPI(baseConf)              // 初始化api部分

	utils.CloseTerminal() // 安全退出
}
