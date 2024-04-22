package core

import (
	"github.com/gin-gonic/gin"
)

func setJsRpcRouters(router *gin.Engine) {
	// 核心部分的的路由组
	router.GET("/", index)

	page := router.Group("/page")
	{
		page.GET("/cookie", GetCookie)
		page.GET("/html", GetHtml)
	}

	rpc := router.Group("/")
	{
		rpc.GET("go", getResult)
		rpc.POST("go", getResult)
		rpc.GET("ws", ws)
		rpc.GET("wst", wsTest)
		rpc.GET("execjs", execjs)
		rpc.POST("execjs", execjs)
		rpc.GET("list", getList)
	}

}
