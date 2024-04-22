package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/unrolled/secure"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var (
	upGrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	gm        = &sync.Mutex{}
	hlSyncMap sync.Map
)

// Message 请求和传递请求
type Message struct {
	Action string `json:"action"`
	Param  string `json:"param"`
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
	actionData  map[string]chan string
	clientWs    *websocket.Conn
}

// NewClient  initializes a new Clients instance
func NewClient(group string, uid string, ws *websocket.Conn) *Clients {
	return &Clients{
		clientGroup: group,
		clientId:    uid,
		actionData:  make(map[string]chan string, 1), // action有消息后就保存到chan里
		clientWs:    ws,
	}
}

func GinJsonMsg(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"status": code, "data": msg})
	return
}

// ws, provides inject function for a job
func ws(c *gin.Context) {
	group, clientId := c.Query("group"), c.Query("clientId")
	//必须要group名字，不然不让它连接ws
	if group == "" {
		return
	}
	//没有给客户端id的话 就用时间戳给他生成一个
	if clientId == "" {
		clientId = utils.GetUUID()
	}
	wsClient, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("websocket err:", err)
		return
	}
	client := NewClient(group, clientId, wsClient)
	hlSyncMap.Store(group+"->"+clientId, client)
	utils.LogPrint("新上线group:" + group + ",clientId:->" + clientId)
	for {
		//等待数据
		_, message, err := wsClient.ReadMessage()
		if err != nil {
			break
		}
		msg := string(message)
		check := []uint8{104, 108, 94, 95, 94}
		strIndex := strings.Index(msg, string(check))
		if strIndex >= 1 {
			action := msg[:strIndex]
			client.actionData[action] <- msg[strIndex+5:]
			if len(msg) > 100 {
				utils.LogPrint("get_message:", msg[strIndex+5:101]+"......")
			} else {
				utils.LogPrint("get_message:", msg[strIndex+5:])
			}

		} else {
			log.Error(msg, "message error")
		}

	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
		utils.LogPrint(group+"->"+clientId, "下线了")
		hlSyncMap.Range(func(key, value interface{}) bool {
			//client, _ := value.(*Clients)
			if key == group+"->"+clientId {
				hlSyncMap.Delete(key)
			}
			return true
		})
	}(wsClient)
}

func wsTest(c *gin.Context) {
	testClient, _ := upGrader.Upgrade(c.Writer, c.Request, nil)
	for {
		//等待数据
		_, message, err := testClient.ReadMessage()
		if err != nil {
			break
		}
		msg := string(message)
		utils.LogPrint("接收到测试消息", msg)
		_ = testClient.WriteMessage(websocket.BinaryMessage, []byte(msg))
	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
	}(testClient)
}

func GetCookie(c *gin.Context) {
	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	group := c.Query("group")
	if group == "" {
		GinJsonMsg(c, http.StatusBadRequest, "需要传入group")
		return
	}

	clientId := RequestParam.ClientId
	client := getRandomClient(group, clientId)
	if client == nil {
		GinJsonMsg(c, http.StatusBadRequest, "没有找到对应的group或clientId,请通过list接口查看现有的注入")
		return
	}

	c3 := make(chan string, 1)
	go client.GQueryFunc("_execjs", utils.ConcatCode("document.cookie"), c3)
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c3})
}

func GetHtml(c *gin.Context) {
	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	group := c.Query("group")
	if group == "" {
		GinJsonMsg(c, http.StatusBadRequest, "需要传入group")
		return
	}

	clientId := RequestParam.ClientId
	client := getRandomClient(group, clientId)
	if client == nil {
		GinJsonMsg(c, http.StatusBadRequest, "没有找到对应的group或clientId,请通过list接口查看现有的注入")
		return
	}

	c3 := make(chan string, 1)
	go client.GQueryFunc("_execjs", utils.ConcatCode("document.documentElement.outerHTML"), c3)
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c3})
}

// GetResult 接收web请求参数，并发给客户端获取结果
func getResult(c *gin.Context) {
	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	group := RequestParam.GroupName
	if group == "" {
		GinJsonMsg(c, http.StatusBadRequest, "需要传入group")
		return
	}
	action := RequestParam.Action
	if action == "" {
		GinJsonMsg(c, http.StatusOK, "请传入action来调用客户端方法")
		return
	}
	clientId := RequestParam.ClientId
	client := getRandomClient(group, clientId)
	if client == nil {
		GinJsonMsg(c, http.StatusBadRequest, "没有找到对应的group或clientId,请通过list接口查看现有的注入")
		return
	}
	c2 := make(chan string, 1)
	go client.GQueryFunc(action, RequestParam.Param, c2)
	//把管道传过去，获得值就返回了
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c2})

}

func execjs(c *gin.Context) {
	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}
	Action := "_execjs"
	//获取参数
	group := RequestParam.GroupName
	if group == "" {
		GinJsonMsg(c, http.StatusBadRequest, "需要传入group")
		return
	}
	JsCode := RequestParam.Code
	if JsCode == "" {
		GinJsonMsg(c, http.StatusBadRequest, "请传入代码")
		return
	}
	clientId := RequestParam.ClientId
	client := getRandomClient(group, clientId)
	if client == nil {
		GinJsonMsg(c, http.StatusBadRequest, "没有找到对应的group或clientId,请通过list接口查看现有的注入")
		return
	}
	c2 := make(chan string)
	go client.GQueryFunc(Action, JsCode, c2)
	c.JSON(200, gin.H{"status": "200", "group": client.clientGroup, "name": client.clientId, "data": <-c2})

}

func getList(c *gin.Context) {
	var data = make(map[string][]string)
	hlSyncMap.Range(func(_, value interface{}) bool {
		client, ok := value.(*Clients)
		if !ok {
			return true // 继续遍历
		}
		group := client.clientGroup
		data[group] = append(data[group], client.clientId)
		return true
	})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": data})
}

func index(c *gin.Context) {
	c.String(200, "你好，我是黑脸怪~")
}

func tlsHandler(HttpsHost string) gin.HandlerFunc {
	return func(c *gin.Context) {
		secureMiddleware := secure.New(secure.Options{
			SSLRedirect: true,
			SSLHost:     HttpsHost,
		})
		err := secureMiddleware.Process(c.Writer, c.Request)
		if err != nil {
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
	return router
}

func InitAPI(conf config.ConfStruct) {
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
	}
	log.Infoln(sb.String())

	err := router.Run(conf.BasicListen)
	if err != nil {
		log.Errorln("服务启动失败..")
	}
}
