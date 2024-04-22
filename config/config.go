package config

import (
	"JsRpc/utils"
	"errors"
	"gopkg.in/yaml.v3"
	"os"
)

var DefaultTimeout = 30

func InitConf(path string) (ConfStruct, error) {
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
