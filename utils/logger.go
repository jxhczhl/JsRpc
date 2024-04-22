package utils

import (
	log "github.com/sirupsen/logrus"
)

var isPrint = true

func InitLogger(closeLog bool) {
	if closeLog {
		isPrint = false
	}
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true, // 强制终端输出带颜色日志
		FullTimestamp:   true, // 显示完整时间戳
		TimestampFormat: "2006-01-02 15:04:05",
		DisableQuote:    true,
	})
}

type logWriter struct{}

func (w logWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func LogPrint(p ...interface{}) {
	if isPrint {
		log.Infoln(p)
	}
}
