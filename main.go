package main

import (
	"JsRpc/config"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/unrolled/secure"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	upGrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	hlSyncMap      sync.Map
	gm             = &sync.Mutex{}
	defaultTimeout = 30
	isPrint        = true
)

// Clients 客户端信息
type Clients struct {
	clientGroup string
	clientId    string
	actionData  map[string]chan string
	clientWs    *websocket.Conn
}

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
}

type logWriter struct{}

func (w logWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// is print?
func logPrint(p ...interface{}) {
	if isPrint {
		log.Infoln(p)
	}
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
		millisId := time.Now().UnixNano() / int64(time.Millisecond)
		clientId = fmt.Sprintf("%d", millisId)
	}
	wsClient, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error("websocket err:", err)
		return
	}
	client := NewClient(group, clientId, wsClient)
	hlSyncMap.Store(group+"->"+clientId, client)
	logPrint("新上线group:" + group + ",clientId:->" + clientId)
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
			logPrint("get_message:", msg[strIndex+5:])
		} else {
			log.Error(msg, "message error")
		}

	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
		logPrint(group+"->"+clientId, "下线了")
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
		logPrint("接收到测试消息", msg)
		_ = testClient.WriteMessage(websocket.BinaryMessage, []byte(msg))
	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
	}(testClient)
}

// GQueryFunc 发送请求到客户端
func GQueryFunc(client *Clients, funcName string, param string, resChan chan<- string) {
	WriteDate := Message{}
	WriteDate.Action = funcName
	if param == "" {
		WriteDate.Param = ""
	} else {
		WriteDate.Param = param
	}
	data, _ := json.Marshal(WriteDate)
	clientWs := client.clientWs
	if client.actionData[funcName] == nil {
		client.actionData[funcName] = make(chan string, 1) //此次action初始化1个消息
	}
	gm.Lock()
	err := clientWs.WriteMessage(1, data)
	gm.Unlock()
	if err != nil {
		fmt.Println(err, "写入数据失败")
	}
	resultFlag := false
	for i := 0; i < defaultTimeout*10; i++ {
		if len(client.actionData[funcName]) > 0 {
			res := <-client.actionData[funcName]
			resChan <- res
			resultFlag = true
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	// 循环完了还是没有数据，那就超时退出
	if true != resultFlag {
		resChan <- "黑脸怪：timeout"
	}
	defer func() {
		close(resChan)
	}()
}

// GetResult 接收web请求参数，并发给客户端获取结果
func GetResult(c *gin.Context) {
	var RequestParam ApiParam
	if err := c.ShouldBind(&RequestParam); err != nil {
		GinJsonMsg(c, http.StatusBadRequest, err.Error())
		return
	}

	group := RequestParam.GroupName
	if group == "" {
		GinJsonMsg(c, http.StatusBadRequest, "需要传入group")
	}
	groupClients := make([]*Clients, 0)
	//循环读取syncMap 获取group名字的
	hlSyncMap.Range(func(_, value interface{}) bool {
		tmpClients, ok := value.(*Clients)
		if !ok {
			return true
		}
		if tmpClients.clientGroup == group {
			groupClients = append(groupClients, tmpClients)
		}
		return true
	})
	if len(groupClients) == 0 {
		GinJsonMsg(c, http.StatusBadRequest, "没有找到注入的group:"+group)
		return
	}
	action := RequestParam.Action
	if action == "" {
		GinJsonMsg(c, http.StatusOK, "请传入action来调用客户端方法")
		return
	}
	clientId := RequestParam.ClientId
	var client *Clients
	// 不传递clientId时候，从group分组随便拿一个
	if clientId == "" {
		// 使用随机数发生器
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		randomIndex := r.Intn(len(groupClients))
		client = groupClients[randomIndex]

	} else {
		clientName, ok := hlSyncMap.Load(group + "->" + clientId)
		if ok == false {
			GinJsonMsg(c, http.StatusBadRequest, "没有找到group,clientId:"+group+"->"+clientId)
			return
		}
		//取一个ws客户端
		client, _ = clientName.(*Clients)

	}
	c2 := make(chan string, 1)
	go GQueryFunc(client, action, RequestParam.Param, c2)
	//把管道传过去，获得值就返回了
	c.JSON(http.StatusOK, gin.H{"status": 200, "group": client.clientGroup, "clientId": client.clientId, "data": <-c2})

}

func Execjs(c *gin.Context) {
	var getGroup, getName, JsCode string
	Action := "_execjs"
	//获取参数
	getGroup, getName, JsCode = c.Query("group"), c.Query("name"), c.Query("jscode")
	//如果获取不到 说明是post提交的
	if getGroup == "" && getName == "" {
		//切换post获取方式
		getGroup, getName, JsCode = c.PostForm("group"), c.PostForm("name"), c.PostForm("jscode")
	}
	if getGroup == "" || getName == "" {
		c.JSON(400, gin.H{"status": 400, "data": "input group and name"})
		return
	}
	logPrint(getGroup, getName, JsCode)
	clientName, ok := hlSyncMap.Load(getGroup + "->" + getName)
	if ok == false {
		c.JSON(400, gin.H{"status": 400, "data": "注入了ws？没有找到当前组和名字"})
		return
	}
	//取一个ws客户端
	client, ko := clientName.(*Clients)
	if !ko {
		return
	}

	c2 := make(chan string)
	go GQueryFunc(client, Action, JsCode, c2)
	c.JSON(200, gin.H{"status": "200", "group": client.clientGroup, "name": client.clientId, "data": <-c2})

}

func GetList(c *gin.Context) {
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

func Index(c *gin.Context) {
	c.String(200, "你好，我是黑脸怪~")
}

func TlsHandler(HttpsHost string) gin.HandlerFunc {
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
func main() {
	JsRpc := "       __       _______..______      .______     ______ \n      |  |     /       ||   _  \\     |   _  \\   /      |\n      |  |    |   (----`|  |_)  |    |  |_)  | |  ,----'\n.--.  |  |     \\   \\    |      /     |   ___/  |  |     \n|  `--'  | .----)   |   |  |\\  \\----.|  |      |  `----.\n \\______/  |_______/    | _| `._____|| _|       \\______|\n                                                        \n"
	fmt.Print(JsRpc)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true, // 强制终端输出带颜色日志
		FullTimestamp:   true, // 显示完整时间戳
		TimestampFormat: "2006-01-02 15:04:05",
	})
	var ConfigPath string
	// 定义命令行参数-c，后面跟着的是默认值以及参数说明
	flag.StringVar(&ConfigPath, "c", "config.yaml", "指定配置文件的路径")
	// 解析命令行参数
	flag.Parse()
	MainConf := config.ConfStruct{
		BasicListen: `:12080`,
		HttpsServices: config.HttpsConfig{
			IsEnable:    false,
			HttpsListen: `:12443`,
		},
		DefaultTimeOut: defaultTimeout,
	}
	err := config.InitConf(ConfigPath, &MainConf)
	if err != nil {
		log.Error("读取配置文件错误，将使用默认配置运行。 ", err.Error())
	}
	if MainConf.CloseWebLog {
		// 将默认的日志输出器设置为空
		gin.DefaultWriter = logWriter{}
	}
	if MainConf.CloseLog {
		isPrint = false
	}
	defaultTimeout = MainConf.DefaultTimeOut

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.GET("/", Index)
	r.GET("/go", GetResult)
	r.POST("/go", GetResult)
	r.GET("/ws", ws)
	r.GET("/wst", wsTest)
	r.GET("/execjs", Execjs)
	r.POST("/execjs", Execjs)
	r.GET("/list", GetList)
	if MainConf.HttpsServices.IsEnable {
		r.Use(TlsHandler(MainConf.HttpsServices.HttpsListen))
		go func() {
			err := r.RunTLS(
				MainConf.HttpsServices.HttpsListen,
				MainConf.HttpsServices.PemPath,
				MainConf.HttpsServices.KeyPath,
			)
			if err != nil {
				log.Error(err)
			}
		}()

	}
	var sb strings.Builder
	sb.WriteString("当前监听地址：")
	sb.WriteString(MainConf.BasicListen)

	sb.WriteString(" ssl启用状态：")
	sb.WriteString(strconv.FormatBool(MainConf.HttpsServices.IsEnable))

	if MainConf.HttpsServices.IsEnable {
		sb.WriteString(" https监听地址：")
		sb.WriteString(MainConf.HttpsServices.HttpsListen)
	}
	log.Infoln(sb.String())

	err = r.Run(MainConf.BasicListen)
	if err != nil {
		log.Error(err)
	}
}
