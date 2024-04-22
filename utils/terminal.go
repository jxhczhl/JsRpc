package utils

import (
	"context"
	log "github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func CloseTerminal() {
	// 等待中断信号以优雅地关闭服务器（设置 5 秒的超时时间）
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Println("- EXIT - [Ctrl+C] The project will automatically close after 3 seconds")
	time.Sleep(time.Second * 3)
}
