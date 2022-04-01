package main

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/unrolled/secure"
	"net/http"
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

	// OverTime 设置接口没得到结果时的超时时间
	OverTime = time.Second * 30
)

type Clients struct {
	clientGroup string
	clientName  string
	Data        map[string]string
	clientWs    *websocket.Conn
}

// NewClients initializes a new Clients instance
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
			fmt.Println("get_message:", client.Data[action])
			hlSyncMap.Store(group+"->"+name, client)
		} else {
			fmt.Println(msg, "message error")
		}

	}
	defer func(ws *websocket.Conn) {
		_ = ws.Close()
	}(ws)

}

func QueryFunc(client *Clients, funcName string, param string) {
	var WriteDate string
	if param == "" {
		WriteDate = "{\"action\":\"" + funcName + "\"}"
	} else {
		WriteDate = "{\"action\":\"" + funcName + "\",\"param\":\"" + param + "\"}"
	}
	ws := client.clientWs
	err := ws.WriteMessage(1, []byte(WriteDate))
	if err != nil {
		fmt.Println(err, "写入数据失败")
	}

}

func ResultSet(c *gin.Context) {
	var getGroup, getName, Action, getParam string

	//获取参数
	getGroup, getName, Action, getParam = c.Query("group"), c.Query("name"), c.Query("action"), c.Query("param")
	//如果获取不到 说明是post提交的
	if getGroup == "" && getName == "" {
		//切换post获取方式
		getGroup, getName, Action, getParam = c.PostForm("group"), c.PostForm("name"), c.PostForm("action"), c.PostForm("param")
	}

	Param := strings.Replace(getParam, "\"", "\\\"", -1)
	if getGroup == "" || getName == "" {
		c.String(200, "input group and name")
		return
	}
	clientName, ok := hlSyncMap.Load(getGroup + "->" + getName)
	if ok == false {
		c.String(200, "注入了ws？没有找到当前组和名字")
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
	//发送数据到web里得到结果
	QueryFunc(client, Action, Param)

	ctx, cancel := context.WithTimeout(context.Background(), OverTime)
	for {
		select {
		case <-ctx.Done():
			// 获取数据超时了
			cancel()
			return
		default:
			data := client.Data[Action]
			//fmt.Println("正常中")
			if data != "" {
				cancel()
				//这里设置为空是为了清除上次的结果并且赋值判断
				client.Data[Action] = ""
				c.JSON(200, gin.H{"status": "200", "group": client.clientGroup, "name": client.clientName, Action: data})
			} else {
				time.Sleep(time.Millisecond * 500)
			}
		}
	}

}

func getList(c *gin.Context) {
	resList := "hliang:\r\n"
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
	r := gin.Default()
	r.GET("/", Index)
	r.GET("/go", ResultSet)
	r.POST("/go", ResultSet)
	r.GET("/ws", ws)
	r.GET("/list", getList)
	_ = r.Run(BasicPort)

	//编译https版放开下面两行注释代码 并且把上一行注释
	//r.Use(TlsHandler())
	//_ = r.RunTLS(SSLPort, "zhengshu.pem", "zhengshu.key")

}
