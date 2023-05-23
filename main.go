package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/unrolled/secure"
	"net/http"
	"os"
	"strings"
	"sync"
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
	hlSyncMap      sync.Map
	gm             = &sync.Mutex{}
	gchan, isPrint = make(chan string), false
)

type Clients struct {
	clientGroup string
	clientName  string
	Data        map[string]string
	clientWs    *websocket.Conn
}

type Message struct {
	Action string `json:"action"`
	Param  string `json:"param"`
}

// is print?
func logPrint(p ...interface{}) {
	if isPrint {
		fmt.Println(p)
	}
}

// NewClient  initializes a new Clients instance
func NewClient(group string, name string, ws *websocket.Conn) *Clients {
	return &Clients{
		clientGroup: group,
		clientName:  name,
		Data:        make(map[string]string),
		clientWs:    ws,
	}
}

// ws, provides inject function for a job
func ws(c *gin.Context) {
	group, name := c.Query("group"), c.Query("name")
	if group == "" || name == "" {
		return
	}
	ws, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("websocket err:", err)
		return
	}
	client := NewClient(group, name, ws)
	hlSyncMap.Store(group+"->"+name, client)
	for {
		//等待数据
		_, message, err := ws.ReadMessage()
		if err != nil {
			break
		}
		msg := string(message)
		check := []uint8{104, 108, 94, 95, 94}
		strIndex := strings.Index(msg, string(check))
		if strIndex >= 1 {
			action := msg[:strIndex]
			client.Data[action] = msg[strIndex+5:]
			logPrint("get_message:", client.Data[action])
			gchan <- msg[strIndex+5:]
			hlSyncMap.Store(group+"->"+name, client)
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
	}(ws)
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
	ws := client.clientWs
	gm.Lock()
	err := ws.WriteMessage(2, data)
	gm.Unlock()
	if err != nil {
		fmt.Println(err, "写入数据失败")
	}
	res := <-gchan
	resChan <- res
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

	c2 := make(chan string)
	go GQueryFunc(client, Action, Param, c2)
	//把管道传过去，获得值就返回了
	c.JSON(200, gin.H{"status": 200, "group": client.clientGroup, "name": client.clientName, "data": <-c2})

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

	_ = r.Run(BasicPort)

	//编译https版放开下面这行注释代码 并且把上一行注释
	//_ = r.RunTLS(SSLPort, "zhengshu.pem", "zhengshu.key")

}
