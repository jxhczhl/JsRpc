package config

import (
	"JsRpc/utils"
	"errors"
	"flag"
	"os"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

var DefaultTimeout = 30

func ReadConf() ConfStruct {
	defer func() {
		if r := recover(); r != nil {
			log.Error("读取配置文件异常: ", r)
		}
	}()

	var ConfigPath string
	// 定义命令行参数-c，后面跟着的是默认值以及参数说明
	flag.StringVar(&ConfigPath, "c", "config.yaml", "指定配置文件的路径")
	// 解析命令行参数
	flag.Parse()

	conf, err := initConf(ConfigPath)
	if err != nil {
		log.Warning(
			"使用默认配置运行 ", err.Error(), "\n",
			"配置参考 https://github.com/jxhczhl/JsRpc/blob/main/config.yaml")
	}
	return conf
}

func initConf(path string) (ConfStruct, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("initConf panic recovered: ", r)
		}
	}()

	defaultConf := ConfStruct{
		BasicListen: `:12080`,
		HttpsServices: HttpsConfig{
			IsEnable:    false,
			HttpsListen: `:12443`,
		},
		DefaultTimeOut: DefaultTimeout,
		RouterReplace: RouterReplace{
			IsEnable:     false,
			ReplaceRoute: "",
		},
	}
	if !utils.IsExists(path) {
		return defaultConf, errors.New("config path not found")
	}

	file, err := os.Open(path)
	if err != nil {
		log.Error("打开配置文件失败: ", err)
		return defaultConf, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Error("关闭配置文件失败: ", err)
		}
	}(file)
	conf := ConfStruct{}
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&conf)
	if err != nil {
		log.Error("解析配置文件失败: ", err)
		return defaultConf, err
	}
	// 设置超时时间
	if conf.DefaultTimeOut > 0 {
		DefaultTimeout = conf.DefaultTimeOut
	}
	return conf, nil
}

type ConfStruct struct {
	BasicListen    string        `yaml:"BasicListen"`
	HttpsServices  HttpsConfig   `yaml:"HttpsServices"`
	DefaultTimeOut int           `yaml:"DefaultTimeOut"`
	CloseLog       bool          `yaml:"CloseLog"`
	CloseWebLog    bool          `yaml:"CloseWebLog"`
	Mode           string        `yaml:"Mode"`
	Cors           bool          `yaml:"Cors"`
	RouterReplace  RouterReplace `yaml:"RouterReplace"`
}

// HttpsConfig 代表HTTPS相关配置的结构体
type HttpsConfig struct {
	IsEnable    bool   `yaml:"IsEnable"`
	HttpsListen string `yaml:"HttpsListen"`
	PemPath     string `yaml:"PemPath"`
	KeyPath     string `yaml:"KeyPath"`
}

type RouterReplace struct {
	IsEnable     bool   `yaml:"IsEnable"`
	ReplaceRoute string `yaml:"ReplaceRoute"`
}
