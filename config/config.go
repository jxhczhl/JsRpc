package config

import (
	"JsRpc/utils"
	"errors"
	"flag"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
)

var DefaultTimeout = 30

func ReadConf() ConfStruct {
	var ConfigPath string
	// 定义命令行参数-c，后面跟着的是默认值以及参数说明
	flag.StringVar(&ConfigPath, "c", "config.yaml", "指定配置文件的路径")
	// 解析命令行参数
	flag.Parse()

	conf, err := initConf(ConfigPath)
	if err != nil {
		log.Warning("读取配置文件错误，将使用默认配置运行。 ", err.Error())
	}
	return conf
}

func initConf(path string) (ConfStruct, error) {
	defaultConf := ConfStruct{
		BasicListen: `:12080`,
		HttpsServices: HttpsConfig{
			IsEnable:    false,
			HttpsListen: `:12443`,
		},
		DefaultTimeOut: DefaultTimeout,
	}
	if !utils.IsExists(path) {
		return defaultConf, errors.New("config path not found")
	}

	file, _ := os.Open(path) // 因为上面已经判断了 文件是存在的 所以这里不用捕获错误
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
		}
	}(file)
	conf := ConfStruct{}
	decoder := yaml.NewDecoder(file)
	err := decoder.Decode(&conf)
	if err != nil {
		return defaultConf, err
	}
	DefaultTimeout = conf.DefaultTimeOut
	return conf, nil
}

type ConfStruct struct {
	BasicListen    string      `yaml:"BasicListen"`
	HttpsServices  HttpsConfig `yaml:"HttpsServices"`
	DefaultTimeOut int         `yaml:"DefaultTimeOut"`
	CloseLog       bool        `yaml:"CloseLog"`
	CloseWebLog    bool        `yaml:"CloseWebLog"`
	Mode           string      `yaml:"Mode"`
	Cors           bool        `yaml:"Cors"`
}

// HttpsConfig 代表HTTPS相关配置的结构体
type HttpsConfig struct {
	IsEnable    bool   `yaml:"IsEnable"`
	HttpsListen string `yaml:"HttpsListen"`
	PemPath     string `yaml:"PemPath"`
	KeyPath     string `yaml:"KeyPath"`
}
