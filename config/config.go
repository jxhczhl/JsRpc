package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

func InitConf(path string, conf *ConfStruct) (err error) {
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err = yaml.Unmarshal(fileContent, &conf); err != nil {
		return
	}
	return
}

type ConfStruct struct {
	BasicListen    string      `yaml:"BasicListen"`
	HttpsServices  HttpsConfig `yaml:"HttpsServices"`
	DefaultTimeOut int         `yaml:"DefaultTimeOut"`
	CloseLog       bool        `yaml:"CloseLog"`
	CloseWebLog    bool        `yaml:"CloseWebLog"`
}

// HttpsConfig 代表HTTPS相关配置的结构体
type HttpsConfig struct {
	IsEnable    bool   `yaml:"IsEnable"`
	HttpsListen string `yaml:"HttpsListen"`
	PemPath     string `yaml:"PemPath"`
	KeyPath     string `yaml:"KeyPath"`
}
