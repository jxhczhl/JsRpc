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

var LocalPort = ":12080"
var sslPort = ":12443"

//设置接口没得到结果时的超时时间
var OverTime = time.Second * 20

type Clients struct {
	clientGroup string
	clientName  string
	//Action      map[string]string
	Data     map[string]string
	clientWs *websocket.Conn
}

var upGrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var hlClients sync.Map

func NewClient(group string, name string, ws *websocket.Conn) *Clients {

	client := &Clients{
		clientGroup: group,
		clientName:  name,
		Data:        make(map[string]string),
		//Action:      make(map[string]string),
		clientWs: ws,
	}
	return client

}

func ws(c *gin.Context) {
	getGroup, getName := c.Query("group"), c.Query("name")
	if getGroup == "" || getName == "" {
		return
	}
	ws, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("websocket err:", err)
		return
	}
	client := NewClient(getGroup, getName, ws)
	//message := []byte("hello," + getGroup + "->" + getName)
	//err = ws.WriteMessage(1, message)
	hlClients.Store(getGroup+"->"+getName, client)
	//defer ws.Close()
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
			//fmt.Println(action,"save msg")
			//if client.Data[action] == "" {
			//	client.Data[action] = msg[strIndex+5:]
			//}
			client.Data[action] = msg[strIndex+5:]
			fmt.Println("get_message:", client.Data[action])
			hlClients.Store(getGroup+"->"+getName, client)
		} else {
			fmt.Println(msg, "message error")
		}

	}
	defer ws.Close()

}

func QueryFunc(client *Clients, funcName string, param string) {
	var WriteDate string
	if param == "" {
		WriteDate = "{\"action\":\"" + funcName + "\"}"
	} else {
		WriteDate = "{\"action\":\"" + funcName + "\",\"param\":\"" + param + "\"}"
	}
	//fmt.Println(WriteDate, "writeDate")
	ws := client.clientWs
	err := ws.WriteMessage(1, []byte(WriteDate))
	if err != nil {
		fmt.Println(err, "写入数据失败")
	}

}

func Go(c *gin.Context) {
	getGroup, getName, Action, getParam := c.Query("group"), c.Query("name"), c.Query("action"), c.Query("param")
	if getGroup == "" || getName == "" {
		c.String(200, "input group and name")
		return
	}
	//fmt.Println(getGroup, getName)
	clientName, ok := hlClients.Load(getGroup + "->" + getName)
	//fmt.Println(clientName, "clientName")
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
	QueryFunc(client, Action, getParam)
	//time.Sleep(time.Second)
	//data:=value.Action[getAction]
	ctx, cancel := context.WithTimeout(context.Background(), OverTime)
	for {
		select {
		case <-ctx.Done():
			// 获取链接超时了
			//fmt.Println("超时？")
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

			//else {
			//	c.JSON(666, gin.H{"message": "?"})
			//}
		}
	}

}

func getList(c *gin.Context) {
	resList := "hliang:\r\n"
	hlClients.Range(func(key, value interface{}) bool {
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
			SSLHost:     sslPort,
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
	//设置获取数据的超时时间30秒
	r := gin.Default()
	r.GET("/", Index)
	r.GET("/go", Go)
	r.GET("/ws", ws)
	r.GET("/list", getList)
	r.Use(TlsHandler())
	_ = r.Run(LocalPort)
	//_ = r.RunTLS(sslPort, "zhengshu.pem", "zhengshu.key")

}
