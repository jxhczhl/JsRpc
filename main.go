package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/unrolled/secure"
	"net/http"
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
)

var hlSyncMap sync.Map

// Clients provides Connect instance for a job
type Clients struct {
	clientGroup string
	clientName  string
	//Action      map[string]string
	Data     map[string]chan string
	clientWs *websocket.Conn
}

// NewClients initializes a new Clients instance
func NewClients(clientGroup string, clientName string, clientWs *websocket.Conn) *Clients {
	return &Clients{
		clientGroup: clientGroup,
		clientName:  clientName,
		clientWs:    clientWs,
	}
}

// QueryFunc Provides context-sensitive methods
func QueryFunc(client *Clients, funcName string, param string) {
	var WriteDate string
	if param == "" {
		WriteDate = "{\"action\":\"" + funcName + "\"}"
	} else {
		WriteDate = "{\"action\":\"" + funcName + "\",\"param\":\"" + param + "\"}"
	}
	fmt.Println(WriteDate)
	ws := client.clientWs
	err := ws.WriteMessage(1, []byte(WriteDate))
	if err != nil {
		fmt.Println(err)
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
			//fmt.Println(action,"save msg")
			if client.Data[action] == nil {
				client.Data[action] = make(chan string, 1)

			}
			client.Data[action] <- msg[strIndex+5:]

			hlSyncMap.Store(group+"->"+name, client)
		} else {
			fmt.Println(msg)
		}

	}
	hlSyncMap.Delete(group + "->" + name)
	defer ws.Close()
}

// ResultSet provides get result function for a job
// you can use it to Remote operation and get results
func ResultSet(c *gin.Context) {
	//group, name, action, param := c.Query("group"), c.Query("name"), c.Query("action"), c.Query("param")
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
	r.GET("/result", ResultSet)
	r.GET("/ws", ws)
	r.GET("/list", ClientConnectionList)
	r.Use(TlsHandler())
	//r.Run(LocalPort)
	r.RunTLS(SSLPort, "zhengshu.pem", "zhengshu.key")

}
