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
)

// Clients provides Connect instance for a job
type Clients struct {
	clientGroup string
	clientName  string
	Data        map[string]string
	clientWs    *websocket.Conn
}

// NewClients initializes a new Clients instance
func NewClients(clientGroup string, clientName string, clientWs *websocket.Conn) *Clients {
	return &Clients{
		clientGroup: clientGroup,
		clientName:  clientName,
		clientWs:    clientWs,
	}
}

var hlClients sync.Map

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
	client := NewClients(group, name, ws)
	hlSyncMap.Store(group+"->"+name, client)
	for {
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
			hlClients.Store(getGroup+"->"+getName, client)
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

func Go(c *gin.Context) {
	getGroup, getName, Action, getParam := c.Query("group"), c.Query("name"), c.Query("action"), c.Query("param")
	if getGroup == "" || getName == "" {
		c.String(200, "input group and name")
		return
	}
	clientName, ok := hlClients.Load(getGroup + "->" + getName)
=======
	hlSyncMap.Delete(group + "->" + name)
	defer ws.Close()
}

// ResultSet provides get result function for a job
// you can use it to Remote operation and get results
func ResultSet(c *gin.Context) {
	group := c.Query("group")
	name := c.Query("name")
	action := c.Query("action")
	param := c.Query("param")

	if group == "" || name == "" {
		c.String(200, "input group and name")
		return
	}
	fmt.Println(group + "->" + name)
	clientName, ok := hlSyncMap.Load(group + "->" + name)
	fmt.Println(clientName)
	if ok == false {
		c.String(200, "注入了ws？没有找到当前组和名字")
		return
	}
	if action == "" {
		c.JSON(200, gin.H{"group": group, "name": name})
		return
	}

	value, ko := clientName.(*Clients)
	if value.Data[action] == nil {
		value.Data[action] = make(chan string, 1)
	}
	QueryFunc(value, action, param)
	data := <-value.Data[action]

	if ko {
		c.JSON(200, gin.H{"status": "200", "group": value.clientGroup, "name": value.clientName, action: data})
	} else {
		c.JSON(666, gin.H{"message": "?"})
	}

}

// ClientConnectionList provides get client connect list for a job
// you can use it see all Connection
func ClientConnectionList(c *gin.Context) {
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
	//设置获取数据的超时时间30秒
	r := gin.Default()
	r.GET("/result", ResultSet)
	r.GET("/ws", ws)
	r.GET("/list", ClientConnectionList)
	r.Use(TlsHandler())
	_ = r.Run(LocalPort)
	//_ = r.RunTLS(sslPort, "zhengshu.pem", "zhengshu.key")
	r.Run(BasicPort)
	//r.RunTLS(SSLPort, "zhengshu.pem", "zhengshu.key")
}
