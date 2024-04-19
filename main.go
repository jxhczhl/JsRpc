package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/unrolled/secure"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	// BasicPort The original port without SSL certificate
	BasicPort = `:12080`
	// SSLPort "Secure" port with SSL certificate
	SSLPort = `:12443`
	// websocket.Upgrader specifies parameters for upgrading an HTTP connection to a
	// WebSocket connection.
	upGrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	hlSyncMap sync.Map
	gm        = &sync.Mutex{}
	// 默认超时时间，没有得到数据的超时时间 单位：秒
	defaultTimeout = 30
	isPrint        = false
)

type Clients struct {
	clientGroup string
	clientName  string
	actionData  map[string]chan string
	clientWs    *websocket.Conn
}

type Message struct {
	Action string `json:"action"`
	Param  string `json:"param"`
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
func NewClient(group string, name string, ws *websocket.Conn) *Clients {
	return &Clients{
		clientGroup: group,
		clientName:  name,
		actionData:  make(map[string]chan string, 1), // action有消息后就保存到chan里
		clientWs:    ws,
	}
}

// ws, provides inject function for a job
func ws(c *gin.Context) {
	group, name := c.Query("group"), c.Query("name")
	if group == "" || name == "" {
		return
	}
	wsClient, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("websocket err:", err)
		return
	}
	client := NewClient(group, name, wsClient)
	hlSyncMap.Store(group+"->"+name, client)
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
			//hlSyncMap.Store(group+"->"+name, client)
		} else {
			fmt.Println(msg, "message error")
		}

	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
		logPrint(group+"->"+name, "下线了")
		hlSyncMap.Range(func(key, value interface{}) bool {
			//client, _ := value.(*Clients)
			if key == group+"->"+name {
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
		_ = testClient.WriteMessage(1, []byte(msg))
	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
	}(testClient)
}

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

func ResultSet(c *gin.Context) {
	var getGroup, getName, Action, Param string

	//获取参数
	getGroup, getName, Action, Param = c.Query("group"), c.Query("name"), c.Query("action"), c.Query("param")
	//如果获取不到 说明是post提交的
	if getGroup == "" && getName == "" {
		//切换post获取方式
		getGroup, getName, Action, Param = c.PostForm("group"), c.PostForm("name"), c.PostForm("action"), c.PostForm("param")
	}

	if getGroup == "" || getName == "" {
		c.JSON(400, gin.H{"status": 400, "data": "input group and name"})
		return
	}
	clientName, ok := hlSyncMap.Load(getGroup + "->" + getName)
	if ok == false {
		c.JSON(400, gin.H{"status": 400, "data": "注入了ws？没有找到当前组和名字"})
		return
	}
	if Action == "" {
		c.JSON(200, gin.H{"group": getGroup, "name": getName})
		return
	}
	//取一个ws客户端
	client, ko := clientName.(*Clients)
	if !ko {
		return
	}

	c2 := make(chan string, 1)
	go GQueryFunc(client, Action, Param, c2)
	//把管道传过去，获得值就返回了
	c.JSON(200, gin.H{"status": 200, "group": client.clientGroup, "name": client.clientName, "data": <-c2})

}

func checkTimeout(c2 chan string) {
	// 100ms检查一次
	for i := 0; i < defaultTimeout*10; i++ {
		if len(c2) > 0 {
			return
		}
		time.Sleep(time.Millisecond * 100)
	}
	// 循环完了还是没有数据，那就超时退出
	c2 <- "黑脸怪：timeout"
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
	c.JSON(200, gin.H{"status": "200", "group": client.clientGroup, "name": client.clientName, "data": <-c2})

}

func getList(c *gin.Context) {
	resList := "黑脸怪:\r\n\t"
	hlSyncMap.Range(func(key, value interface{}) bool {
		resList += key.(string) + "\r\n\t"
		return true
	})
	c.String(200, resList)
}

func Index(c *gin.Context) {
	c.String(200, "你好，我是黑脸怪~")
}

func TlsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		secureMiddleware := secure.New(secure.Options{
			SSLRedirect: true,
			SSLHost:     SSLPort,
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
	for _, v := range os.Args {
		if v == "log" {
			isPrint = true
		}
	}
	// 将默认的日志输出器设置为空
	//gin.DefaultWriter = logWriter{}
	fmt.Println("欢迎使用jsrpc~")
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.GET("/", Index)
	r.GET("/go", ResultSet)
	r.POST("/go", ResultSet)
	r.GET("/ws", ws)
	r.GET("/wst", wsTest)
	r.GET("/execjs", Execjs)
	r.POST("/execjs", Execjs)
	r.GET("/list", getList)
	r.Use(TlsHandler())

	//编译https版放开下面这行注释代码
	//go func() {
	//	err := r.RunTLS(SSLPort, "zhengshu.pem", "zhengshu.key")
	//	if err != nil {
	//		fmt.Println(err)
	//	}
	//}()
	_ = r.Run(BasicPort)

}
