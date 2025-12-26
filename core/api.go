package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/unrolled/secure"
)

var (
	upGrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	rwMu      sync.RWMutex
	hlSyncMap sync.Map
)

// Message 请求和传递请求
type Message struct {
	Action    string `json:"action"`
	MessageId string `json:"message_id"`
	Param     string `json:"param"`
}
type MessageResponse struct {
	Action       string `json:"action"`
	MessageId    string `json:"message_id"`
	ResponseData string `json:"response_data"`
}
type ApiParam struct {
	GroupName string `form:"group" json:"group"`
	ClientId  string `form:"clientId" json:"clientId"`
	Action    string `form:"action" json:"action"`
	Param     string `form:"param" json:"param"`
	Code      string `form:"code" json:"code"` // 直接eval的代码
}

// Clients 客户端信息
type Clients struct {
	clientGroup string
	clientId    string
	clientIp    string                            // 客户端ip
	actionData  map[string]map[string]chan string // {"action":{"消息id":消息管道}}
	clientWs    *websocket.Conn
}

func (c *Clients) readFromMap(funcName string, MessageId string) chan string {
	rwMu.RLock()
	defer rwMu.RUnlock()
	return c.actionData[funcName][MessageId]
}
func (c *Clients) writeToMap(funcName string, MessageId string, msg string) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("写入管道失败 (可能已关闭): ", r)
		}
	}()
	rwMu.Lock()
	defer rwMu.Unlock()
	c.actionData[funcName][MessageId] <- msg
}

// NewClient  initializes a new Clients instance
func NewClient(group string, uid string, ws *websocket.Conn, clientIp string) *Clients {
	return &Clients{
		clientGroup: group,
		clientId:    uid,
		actionData:  make(map[string]map[string]chan string), // action有消息后就保存到chan里
		clientWs:    ws,
		clientIp:    clientIp,
	}
}

func GinJsonMsg(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"status": code, "data": msg})
	return
}

// ws, provides inject function for a job
func ws(c *gin.Context) {
	// 添加panic恢复机制
	defer func() {
		if r := recover(); r != nil {
			log.Error("ws handler panic recovered: ", r)
		}
	}()

	group, clientId := c.Query("group"), c.Query("clientId")
	//必须要group名字，不然不让它连接ws
	if group == "" {
		log.Warning("ws连接缺少group参数")
		return
	}
	clientIP := c.ClientIP()

	//没有给客户端id的话 就用uuid给他生成一个
	if clientId == "" {
		clientId = utils.GetUUID()
	}
	wsClient, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("websocket upgrade err:", err)
		return
	}

	// 确保WebSocket连接最终被关闭
	defer func() {
		if wsClient != nil {
			_ = wsClient.Close()
			utils.LogPrint(group+"->"+clientId, "下线了")
			hlSyncMap.Delete(group + "->" + clientId)
		}
	}()

	client := NewClient(group, clientId, wsClient, clientIP)
	hlSyncMap.Store(group+"->"+clientId, client)
	utils.LogPrint("新上线group:" + group + ",clientId:->" + clientId)
	clientNameJson := `{"registerId":"` + clientId + `"}`
	err = wsClient.WriteMessage(1, []byte(clientNameJson))
	if err != nil {
		log.Warning("注册成功，但发送回执信息失败: ", err)
		return
	}
	for {
		//等待数据
		_, message, err := wsClient.ReadMessage()
		if err != nil {
			log.Debug("读取websocket消息失败，连接可能已断开: ", err)
			break
		}
		// 将得到的数据转成结构体
		messageStruct := MessageResponse{}
		err = json.Unmarshal(message, &messageStruct)
		if err != nil {
			log.Error("当前IP：", clientIP, " 接收到的消息不是设定的格式，不做处理: ", err)
			continue
		}
		action := messageStruct.Action
		messageId := messageStruct.MessageId
		msg := messageStruct.ResponseData
		// 这里直接给管道塞数据，那么之前发送的时候要初始化好
		if client.readFromMap(action, messageId) == nil {
			log.Warning("当前IP：", clientIP, "当前消息id：", messageId, " 已被超时释放，回调的数据不做处理")
		} else {
			client.writeToMap(action, messageId, msg)
		}
		if len(msg) > 100 {
			utils.LogPrint("id:", messageId, " get_message:", msg[:101]+"......")
		} else {
			utils.LogPrint("IP:", clientIP, " id:", messageId, " get_message:", msg)
		}
	}
}

func wsTest(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("wsTest handler panic recovered: ", r)
		}
	}()

	testClient, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("wsTest upgrade失败: ", err)
		return
	}

	// 确保连接关闭
	defer func() {
		if testClient != nil {
			_ = testClient.Close()
		}
	}()

	for {
		//等待数据
		_, message, err := testClient.ReadMessage()
		if err != nil {
			log.Debug("wsTest读取消息失败: ", err)
			break
		}
		msg := string(message)
		utils.LogPrint("接收到测试消息", msg)
		err = testClient.WriteMessage(websocket.TextMessage, []byte(msg))
		if err != nil {
			log.Error("wsTest发送消息失败: ", err)
			break
		}
	}
}

func checkRequestParam(c *gin.Context) (*Clients, string) {
	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		return &Clients{}, err.Error()
	}
	group := RequestParam.GroupName
	if group == "" {
		return &Clients{}, "需要传入group"
	}
	clientId := RequestParam.ClientId
	client := getRandomClient(group, clientId)
	if client == nil {
		return &Clients{}, "没有找到对应的group或clientId,请通过list接口查看现有的注入"
	}
	return client, ""
}

func GetCookie(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("GetCookie handler panic recovered: ", r)
			GinJsonMsg(c, http.StatusInternalServerError, "服务器内部错误")
		}
	}()

	client, errorStr := checkRequestParam(c)
	if errorStr != "" {
		GinJsonMsg(c, http.StatusBadRequest, errorStr)
		return
	}
	c3 := make(chan string, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("GetCookie goroutine panic recovered: ", r)
				c3 <- "获取cookie失败：内部错误"
			}
		}()
		client.GQueryFunc("_execjs", utils.ConcatCode("document.cookie"), c3, client.clientId)
	}()
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c3})
}

func GetHtml(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("GetHtml handler panic recovered: ", r)
			GinJsonMsg(c, http.StatusInternalServerError, "服务器内部错误")
		}
	}()

	client, errorStr := checkRequestParam(c)
	if errorStr != "" {
		GinJsonMsg(c, http.StatusBadRequest, errorStr)
		return
	}
	c3 := make(chan string, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("GetHtml goroutine panic recovered: ", r)
				c3 <- "获取html失败：内部错误"
			}
		}()
		client.GQueryFunc("_execjs", utils.ConcatCode("document.documentElement.outerHTML"), c3, client.clientId)
	}()
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c3})
}

// GetResult 接收web请求参数，并发给客户端获取结果
func getResult(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("getResult handler panic recovered: ", r)
			GinJsonMsg(c, http.StatusInternalServerError, "服务器内部错误")
		}
	}()

	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	action := RequestParam.Action
	if action == "" {
		GinJsonMsg(c, http.StatusOK, "请传入action来调用客户端方法")
		return
	}
	client, errorStr := checkRequestParam(c)
	if errorStr != "" {
		GinJsonMsg(c, http.StatusBadRequest, errorStr)
		return
	}
	c2 := make(chan string, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("getResult goroutine panic recovered: ", r)
				c2 <- "调用失败：内部错误"
			}
		}()
		client.GQueryFunc(action, RequestParam.Param, c2, client.clientIp)
	}()
	//把管道传过去，获得值就返回了
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c2})
}

func execjs(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("execjs handler panic recovered: ", r)
			GinJsonMsg(c, http.StatusInternalServerError, "服务器内部错误")
		}
	}()

	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	Action := "_execjs"
	//获取参数

	JsCode := RequestParam.Code
	if JsCode == "" {
		GinJsonMsg(c, http.StatusBadRequest, "请传入代码")
		return
	}
	client, errorStr := checkRequestParam(c)
	if errorStr != "" {
		GinJsonMsg(c, http.StatusBadRequest, errorStr)
		return
	}
	c2 := make(chan string, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("execjs goroutine panic recovered: ", r)
				c2 <- "执行js代码失败：内部错误"
			}
		}()
		client.GQueryFunc(Action, JsCode, c2, client.clientIp)
	}()
	c.JSON(200, gin.H{"status": "200", "group": client.clientGroup, "name": client.clientId, "data": <-c2})
}

func getList(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("getList handler panic recovered: ", r)
			GinJsonMsg(c, http.StatusInternalServerError, "服务器内部错误")
		}
	}()

	var data = make(map[string][]string)
	hlSyncMap.Range(func(_, value interface{}) bool {
		client, ok := value.(*Clients)
		if !ok {
			log.Warning("类型断言失败：无法将value转换为*Clients")
			return true // 继续遍历
		}
		group := client.clientGroup
		data[group] = append(data[group], client.clientId)
		return true
	})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": data})
}

func index(c *gin.Context) {
	//c.String(200, "你好，我是黑脸怪~")
	htmlContent := `
		<!DOCTYPE html>
		<html>
		<head><title>欢迎使用JsRpc</title></head>
		<body>
			你好，我是黑脸怪~
			<p>微信：hl98_cn</p>
		</body>
		</html>
		`
	// 返回 HTML 页面
	c.Data(200, "text/html; charset=utf-8", []byte(htmlContent))
}

func tlsHandler(HttpsHost string) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("tlsHandler panic recovered: ", r)
			}
		}()

		secureMiddleware := secure.New(secure.Options{
			SSLRedirect: true,
			SSLHost:     HttpsHost,
		})
		err := secureMiddleware.Process(c.Writer, c.Request)
		if err != nil {
			log.Error("TLS处理失败: ", err)
			c.Abort()
			return
		}
		c.Next()
	}
}

func getGinMode(mode string) string {
	switch mode {
	case "release":
		return gin.ReleaseMode
	case "debug":
		return gin.DebugMode
	case "test":
		return gin.TestMode
	}
	return gin.ReleaseMode // 默认就是release模式
}

func setupRouters(conf config.ConfStruct) *gin.Engine {
	router := gin.Default()
	if conf.Cors { // 是否开启cors中间件
		router.Use(CorsMiddleWare())
	}
	if conf.RouterReplace.IsEnable {
		router.Use(RouteReplace(router, conf.RouterReplace.ReplaceRoute))
	}
	return router
}

func InitAPI(conf config.ConfStruct) {
	defer func() {
		if r := recover(); r != nil {
			log.Fatal("InitAPI panic: ", r)
		}
	}()

	if conf.CloseWebLog {
		// 将默认的日志输出器设置为空
		gin.DefaultWriter = utils.LogWriter{}
	}
	gin.SetMode(getGinMode(conf.Mode))
	router := setupRouters(conf)

	setJsRpcRouters(router) // 核心路由

	var sb strings.Builder
	sb.WriteString("当前监听地址：")
	sb.WriteString(conf.BasicListen)

	sb.WriteString(" ssl启用状态：")
	sb.WriteString(strconv.FormatBool(conf.HttpsServices.IsEnable))

	if conf.HttpsServices.IsEnable {
		sb.WriteString(" https监听地址：")
		sb.WriteString(conf.HttpsServices.HttpsListen)
		router.Use(tlsHandler(conf.HttpsServices.HttpsListen))
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error("HTTPS服务 panic recovered: ", r)
				}
			}()
			err := router.RunTLS(
				conf.HttpsServices.HttpsListen,
				conf.HttpsServices.PemPath,
				conf.HttpsServices.KeyPath,
			)
			if err != nil {
				log.Error("HTTPS服务启动失败: ", err)
			}
		}()
	}
	log.Infoln(sb.String())

	err := router.Run(conf.BasicListen)
	if err != nil {
		log.Fatal("HTTP服务启动失败: ", err)
	}
}
