package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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
	Fuzzy     bool   `form:"fuzzy" json:"fuzzy"`
	Action    string `form:"action" json:"action"`
	Param     string `form:"param" json:"param"`
	Code      string `form:"code" json:"code"` // 直接eval的代码
}

// Clients 客户端信息
type Clients struct {
	clientGroup       string
	clientId          string
	clientIp          string                            // 客户端ip
	actionData        map[string]map[string]chan string // {"action":{"消息id":消息管道}}
	clientWs          *websocket.Conn
	lastPingTime      int64      // 最后一次 ping 成功时间
	failCount         int        // 连续失败次数
	isHealthy         bool       // 是否健康
	wsMu              sync.Mutex // WebSocket 写锁
	registeredActions []string   // 客户端注册的 actions 列表
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
		clientGroup:  group,
		clientId:     uid,
		actionData:   make(map[string]map[string]chan string), // action有消息后就保存到chan里
		clientWs:     ws,
		clientIp:     clientIp,
		lastPingTime: time.Now().Unix(),
		failCount:    0,
		isHealthy:    true,
	}
}

func GinJsonMsg(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"status": code, "data": msg})
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

	// 设置 Pong 处理器
	wsClient.SetPongHandler(func(appData string) error {
		client.lastPingTime = time.Now().Unix()
		//client.isHealthy = true
		// client.failCount = 0 心跳不重置失败计数，获取结果超时后才会重置成可用
		return nil
	})

	// 启动心跳检测 goroutine
	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	go func() {
		ticker := time.NewTicker(15 * time.Second) // 每15秒发送一次心跳
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				client.wsMu.Lock()
				err := wsClient.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				client.wsMu.Unlock()
				if err != nil {
					log.Debug("心跳发送失败: ", err)
					client.isHealthy = false
				}
			}
		}
	}()

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
		// 处理客户端上报的 actions 列表
		if action == "_registerActions" && messageId == "" {
			var actions []string
			if err := json.Unmarshal([]byte(msg), &actions); err == nil {
				client.registeredActions = actions
				utils.LogPrint("客户端", clientId, "注册了actions:", actions)
			}
			continue
		}
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
	client := getRandomClient(group, clientId, RequestParam.Fuzzy)
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

// getClientDetails 获取客户端详细信息（包括健康状态和已用actions）
func getClientDetails(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("getClientDetails handler panic recovered: ", r)
			GinJsonMsg(c, http.StatusInternalServerError, "服务器内部错误")
		}
	}()

	type ClientInfo struct {
		ClientId  string   `json:"clientId"`
		ClientIp  string   `json:"clientIp"`
		IsHealthy bool     `json:"isHealthy"`
		FailCount int      `json:"failCount"`
		Actions   []string `json:"actions"`
	}

	var data = make(map[string][]ClientInfo)
	hlSyncMap.Range(func(_, value interface{}) bool {
		client, ok := value.(*Clients)
		if !ok {
			return true
		}
		// 确保 actions 不为 nil，避免 JSON 序列化为 null
		actions := client.registeredActions
		if actions == nil {
			actions = []string{}
		}
		info := ClientInfo{
			ClientId:  client.clientId,
			ClientIp:  client.clientIp,
			IsHealthy: client.isHealthy,
			FailCount: client.failCount,
			Actions:   actions,
		}
		data[client.clientGroup] = append(data[client.clientGroup], info)
		return true
	})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": data})
}

// kickClient 踢除指定客户端
func kickClient(c *gin.Context) {
	group := c.Query("group")
	clientId := c.Query("clientId")

	if group == "" || clientId == "" {
		GinJsonMsg(c, http.StatusBadRequest, "group 和 clientId 参数必填")
		return
	}

	key := group + "->" + clientId
	value, ok := hlSyncMap.Load(key)
	if !ok {
		GinJsonMsg(c, http.StatusNotFound, "客户端不存在")
		return
	}

	client, ok := value.(*Clients)
	if !ok {
		GinJsonMsg(c, http.StatusInternalServerError, "客户端类型错误")
		return
	}

	// 关闭 WebSocket 连接，这会触发客户端的 defer 清理逻辑
	client.clientWs.Close()
	utils.LogPrint("踢除客户端: " + key)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "客户端已踢除"})
}

func index(c *gin.Context) {
	htmlContent := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>JsRpc Console</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600&family=Outfit:wght@300;400;500;600&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body {
            overflow-x: hidden;
            overscroll-behavior-x: none;
        }
        :root {
            --bg-primary: #0a0a0f;
            --bg-secondary: #12121a;
            --bg-card: #1a1a24;
            --bg-card-hover: #22222e;
            --accent: #6366f1;
            --accent-glow: rgba(99, 102, 241, 0.3);
            --success: #10b981;
            --warning: #f59e0b;
            --danger: #ef4444;
            --text-primary: #f1f5f9;
            --text-secondary: #94a3b8;
            --text-muted: #64748b;
            --border: rgba(255, 255, 255, 0.06);
        }
        body {
            font-family: 'Outfit', -apple-system, sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            overflow-x: hidden;
        }
        .bg-grid {
            position: fixed;
            inset: 0;
            background-image: 
                linear-gradient(rgba(99, 102, 241, 0.03) 1px, transparent 1px),
                linear-gradient(170deg, rgba(99, 102, 241, 0.03) 1px, transparent 1px);
            background-size: 60px 60px;
            pointer-events: none;
        }
        .bg-glow {
            position: fixed;
            width: 600px;
            height: 600px;
            background: radial-gradient(circle, var(--accent-glow) 0%, transparent 70%);
            top: -200px;
            right: -200px;
            pointer-events: none;
            animation: pulse 8s ease-in-out infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 0.5; transform: scale(1); }
            50% { opacity: 0.8; transform: scale(1.1); }
        }
        .container {
            max-width: 1400px;
            width: 100%;
            margin: 0 auto;
            padding: 40px 24px;
            position: relative;
            z-index: 1;
            overflow-x: hidden;
        }
        header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 48px;
            padding-bottom: 24px;
            border-bottom: 1px solid var(--border);
        }
        .logo {
            display: flex;
            align-items: center;
            gap: 16px;
        }
        .logo-icon {
            width: 48px;
            height: 48px;
            background: linear-gradient(170deg, var(--accent), #8b5cf6);
            border-radius: 12px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-family: 'JetBrains Mono', monospace;
            font-weight: 600;
            font-size: 20px;
            box-shadow: 0 0 30px var(--accent-glow);
        }
        .logo h1 {
            font-size: 28px;
            font-weight: 600;
            background: linear-gradient(170deg, #fff 0%, var(--text-secondary) 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .logo span {
            font-size: 13px;
            color: var(--text-muted);
            font-weight: 400;
        }
        .status-bar {
            display: flex;
            align-items: center;
            gap: 24px;
        }
        .status-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 14px;
            color: var(--text-secondary);
        }
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--success);
            box-shadow: 0 0 12px var(--success);
            animation: blink 2s ease-in-out infinite;
        }
        @keyframes blink {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 20px;
            margin-bottom: 40px;
        }
        .theme-toggle {
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 8px;
            padding: 8px;
            cursor: pointer;
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
            gap: 6px;
            font-size: 14px;
            color: var(--text-secondary);
        }
        .theme-toggle:hover {
            border-color: var(--accent);
            color: var(--accent);
        }
        .theme-toggle.dark::before { content: '🌙'; }
        .theme-toggle.light::before { content: '☀️'; }
        body.light-theme {
            --bg-primary: #f8fafc;
            --bg-secondary: #ffffff;
            --bg-card: #f1f5f9;
            --bg-card-hover: #e2e8f0;
            --accent: #3b82f6;
            --accent-glow: rgba(59, 130, 246, 0.3);
            --success: #10b981;
            --warning: #f59e0b;
            --danger: #ef4444;
            --text-primary: #1e293b;
            --text-secondary: #475569;
            --text-muted: #64748b;
            --border: rgba(0, 0, 0, 0.1);
        }
        body.light-theme .bg-grid {
            background-image:
                linear-gradient(rgba(59, 130, 246, 0.03) 1px, transparent 1px),
                linear-gradient(90deg, rgba(59, 130, 246, 0.03) 1px, transparent 1px);
        }
        .stat-card {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 16px;
            padding: 24px;
            transition: all 0.3s ease;
        }
        .stat-card:hover {
            background: var(--bg-card-hover);
            border-color: rgba(99, 102, 241, 0.2);
            transform: translateY(-2px);
        }
        .stat-label {
            font-size: 13px;
            color: var(--text-muted);
            letter-spacing: 0.5px;
            margin-bottom: 8px;
        }
        .stat-value {
            font-size: 36px;
            font-weight: 600;
            font-family: 'JetBrains Mono', monospace;
        }
        .stat-value.accent { color: var(--accent); }
        .stat-value.success { color: var(--success); }
        .stat-value.warning { color: var(--warning); }
        .stat-value.danger { color: var(--danger); }
        .main-grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 24px;
        }
        .panel.full-width {
            margin-bottom: 24px;
        }
        .panel {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 16px;
            overflow: hidden;
        }
        .panel-header {
            padding: 20px 24px;
            border-bottom: 1px solid var(--border);
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .panel-title {
            font-size: 16px;
            font-weight: 500;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .panel-title::before {
            content: '';
            width: 4px;
            height: 16px;
            background: var(--accent);
            border-radius: 2px;
        }
        .panel-content {
            padding: 20px 24px;
        }
        .panel-content::-webkit-scrollbar {
            width: 6px;
        }
        .panel-content::-webkit-scrollbar-track {
            background: transparent;
        }
        .panel-content::-webkit-scrollbar-thumb {
            background: var(--border);
            border-radius: 3px;
        }
        .group-section {
            margin-bottom: 24px;
        }
        .group-section:last-child { margin-bottom: 0; }
        .group-name {
            font-size: 12px;
            letter-spacing: 1px;
            color: var(--accent);
            margin-bottom: 12px;
            font-weight: 500;
        }
        .client-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 10px;
            padding: 16px;
            margin-bottom: 10px;
            transition: all 0.2s ease;
        }
        .client-card:hover {
            border-color: rgba(99, 102, 241, 0.3);
        }
        .client-card:last-child { margin-bottom: 0; }
        .clients-grid {
            display: flex;
            flex-direction: column;
            gap: 24px;
        }
        .clients-row {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
            gap: 16px;
        }
        .client-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
            padding-bottom: 12px;
            border-bottom: 1px solid var(--border);
        }
        .client-actions-section {
            margin-top: 12px;
            border-radius: 8px;
            padding: 12px;
        }
        .actions-header {
            font-size: 11px;
            color: var(--text-muted);
            margin-bottom: 10px;
            display: flex;
            align-items: center;
            gap: 6px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .actions-icon {
            font-size: 12px;
            color: var(--warning);
        }
        .no-actions {
            font-size: 12px;
            color: var(--text-muted);
            font-style: italic;
            padding: 8px 0;
        }
        .client-id {
            font-family: 'JetBrains Mono', monospace;
            font-size: 14px;
            font-weight: 500;
        }
        .client-ip {
            font-size: 12px;
            color: var(--text-muted);
        }
        .fail-count {
            font-size: 11px;
            color: var(--danger);
            font-weight: 500;
            margin-top: 4px;
            display: flex;
            align-items: center;
            gap: 4px;
        }
        .health-badge {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 4px 10px;
            border-radius: 20px;
            font-size: 11px;
            font-weight: 500;
            text-transform: uppercase;
        }
        .health-badge.healthy {
            background: rgba(16, 185, 129, 0.15);
            color: var(--success);
        }
        .health-badge.unhealthy {
            background: rgba(239, 68, 68, 0.15);
            color: var(--danger);
        }
        .actions-list {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
        }
        .action-tag {
            background: linear-gradient(170deg, rgba(99, 102, 241, 0.15), rgba(139, 92, 246, 0.1));
            color: var(--accent);
            padding: 6px 14px;
            border-radius: 20px;
            font-size: 12px;
            font-family: 'JetBrains Mono', monospace;
            border: 1px solid rgba(99, 102, 241, 0.2);
            transition: all 0.2s ease;
            position: relative;
            overflow: hidden;
        }
        .action-tag::before {
            content: '⚡';
            margin-right: 6px;
            font-size: 10px;
        }
        .action-tag:hover {
            background: linear-gradient(170deg, rgba(99, 102, 241, 0.25), rgba(139, 92, 246, 0.2));
            border-color: rgba(99, 102, 241, 0.4);
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(99, 102, 241, 0.2);
        }
        .kick-btn {
            background: linear-gradient(170deg, rgba(239, 68, 68, 0.15), rgba(220, 38, 38, 0.1));
            color: var(--danger);
            border: 1px solid rgba(239, 68, 68, 0.3);
            padding: 6px 14px;
            border-radius: 6px;
            font-size: 12px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s ease;
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        .kick-btn:hover {
            background: linear-gradient(170deg, rgba(239, 68, 68, 0.3), rgba(220, 38, 38, 0.2));
            border-color: rgba(239, 68, 68, 0.5);
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(239, 68, 68, 0.2);
        }
        .kick-btn:active {
            transform: translateY(0);
        }
        .kick-btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
            transform: none;
        }
        .client-footer {
            display: flex;
            justify-content: flex-end;
            margin-top: 12px;
            padding-top: 12px;
            border-top: 1px solid var(--border);
        }
        .empty-state {
            text-align: center;
            padding: 48px 24px;
            color: var(--text-muted);
        }
        .empty-icon {
            font-size: 48px;
            margin-bottom: 16px;
            opacity: 0.3;
        }
        .api-list {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }
        .api-item {
            display: flex;
            align-items: center;
            gap: 12px;
            padding: 14px 16px;
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 10px;
            transition: all 0.2s ease;
        }
        .api-item:hover {
            border-color: rgba(99, 102, 241, 0.3);
            background: var(--bg-card-hover);
        }
        .api-method {
            padding: 4px 10px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 600;
            font-family: 'JetBrains Mono', monospace;
        }
        .api-method.get { background: rgba(16, 185, 129, 0.15); color: var(--success); }
        .api-method.post { background: rgba(99, 102, 241, 0.15); color: var(--accent); }
        .api-path {
            font-family: 'JetBrains Mono', monospace;
            font-size: 14px;
            flex: 1;
        }
        .api-desc {
            font-size: 12px;
            color: var(--text-muted);
        }
        .refresh-time {
            font-size: 12px;
            color: var(--text-muted);
        }
        .actions-grid {
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        .action-card {
            background: var(--bg-secondary);
            border: 1px solid var(--border);
            border-radius: 10px;
            padding: 16px;
            transition: all 0.2s ease;
        }
        .action-card:hover {
            border-color: rgba(245, 158, 11, 0.3);
        }
        .action-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        .action-name {
            font-family: 'JetBrains Mono', monospace;
            font-size: 15px;
            font-weight: 600;
            color: var(--warning);
        }
        .action-count {
            font-size: 12px;
            color: var(--text-muted);
            background: var(--bg-card);
            padding: 4px 10px;
            border-radius: 12px;
        }
        .action-clients {
            display: flex;
            flex-wrap: wrap;
            gap: 8px;
        }
        .client-chip {
            display: flex;
            align-items: center;
            gap: 6px;
            background: var(--bg-card);
            padding: 6px 12px;
            border-radius: 6px;
            font-size: 12px;
            font-family: 'JetBrains Mono', monospace;
            color: var(--text-secondary);
        }
        .dot {
            width: 6px;
            height: 6px;
            border-radius: 50%;
        }
        .dot.healthy { background: var(--success); }
        .dot.unhealthy { background: var(--danger); }
        footer {
            margin-top: 48px;
            text-align: center;
            color: var(--text-muted);
            font-size: 13px;
        }
        footer a {
            color: var(--accent);
            text-decoration: none;
        }
        @media (max-width: 1024px) {
            .stats-grid { grid-template-columns: repeat(2, 1fr); }
            .main-grid { grid-template-columns: 1fr; }
            .clients-row { grid-template-columns: 1fr; }
        }
        @media (max-width: 640px) {
            .container { padding: 20px 16px; }
            .stats-grid { grid-template-columns: repeat(2, 1fr); gap: 12px; }
            .stat-card { padding: 16px; }
            .stat-value { font-size: 28px; }
            header { flex-direction: column; gap: 16px; text-align: center; }
            .logo h1 { font-size: 22px; }
            .panel-content { padding: 16px; }
            .client-card { padding: 14px; }
            .action-tag { padding: 5px 10px; font-size: 11px; }
            .action-tag::before { display: none; }
            .client-header { flex-direction: column; align-items: flex-start; gap: 10px; }
            .health-badge { align-self: flex-start; }
            .fail-count { margin-top: 2px; }
        }
        @media (max-width: 400px) {
            .stats-grid { grid-template-columns: 1fr; }
            .container { padding: 16px 12px; }
        }
    </style>
</head>
		<body>
    <div class="bg-grid"></div>
    <div class="bg-glow"></div>
    
    <div class="container">
        <header>
            <div class="logo">
                <div class="logo-icon"></div>
                <div>
                    <h1>JsRpc Console</h1>
                    <span>RPC Server Dashboard</span>
                </div>
            </div>
            <div class="status-bar">
                <div class="status-item refresh-time">
                    Last update: <span id="lastUpdate">-</span>
                </div>
				<button class="theme-toggle dark" id="themeToggle" onclick="toggleTheme()">
                    Toggle Theme
                </button>
            </div>
        </header>

        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-label">Connected Clients</div>
                <div class="stat-value success" id="totalClients">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Healthy Clients</div>
                <div class="stat-value success" id="healthyClients">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Total Groups</div>
                <div class="stat-value accent" id="totalGroups">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Registered Actions</div>
                <div class="stat-value warning" id="totalActions">0</div>
            </div>
        </div>

        <div class="panel full-width">
            <div class="panel-header">
                <div class="panel-title">Connected Clients</div>
            </div>
            <div class="panel-content" id="clientsPanel">
                <div class="empty-state">
                    <div class="empty-icon">📡</div>
                    <div>Loading connections...</div>
                </div>
            </div>
        </div>

        <footer>
            Made with ❤️ by <a href="#">黑脸怪</a> · 微信：hl98_cn
        </footer>
    </div>

    <script>
        // Theme management
        function initTheme() {
            const savedTheme = localStorage.getItem('jsrpc-theme') || 'dark';
            document.body.classList.toggle('light-theme', savedTheme === 'light');
            updateThemeButton(savedTheme);
        }

        function updateThemeButton(theme) {
            const btn = document.getElementById('themeToggle');
            btn.className = 'theme-toggle ' + theme;
            btn.textContent = theme === 'dark' ? ' Toggle Theme' : ' Toggle Theme';
        }

        function toggleTheme() {
            const isLight = document.body.classList.contains('light-theme');
            const newTheme = isLight ? 'dark' : 'light';
            document.body.classList.toggle('light-theme', !isLight);
            localStorage.setItem('jsrpc-theme', newTheme);
            updateThemeButton(newTheme);
        }

        // Initialize theme on load
        initTheme();

        async function fetchData() {
            try {
                const res = await fetch('/details');
                const json = await res.json();
                if (json.status === 200) {
                    renderClients(json.data);
                }
            } catch (e) {
                console.error('Failed to fetch:', e);
            }
            document.getElementById('lastUpdate').textContent = new Date().toLocaleTimeString();
        }

        function renderClients(data) {
            const groups = Object.keys(data);
            let totalClients = 0;
            let healthyClients = 0;
            let allActions = new Set();

            groups.forEach(group => {
                data[group].forEach(client => {
                    totalClients++;
                    if (client.isHealthy) healthyClients++;
                    if (client.actions) {
                        client.actions.forEach(a => allActions.add(a));
                    }
                });
            });

            document.getElementById('totalGroups').textContent = groups.length;
            document.getElementById('totalClients').textContent = totalClients;
            document.getElementById('healthyClients').textContent = healthyClients;
            document.getElementById('totalActions').textContent = allActions.size;

            if (groups.length === 0) {
                document.getElementById('clientsPanel').innerHTML = 
                    '<div class="empty-state"><div class="empty-icon">📡</div><div>No clients connected</div></div>';
                return;
            }

            let html = '<div class="clients-grid">';
            groups.forEach(group => {
                html += '<div class="group-section">';
                html += '<div class="group-name">Group: ' + group + '</div>';
                html += '<div class="clients-row">';
                data[group].forEach(client => {
                    const healthClass = client.isHealthy ? 'healthy' : 'unhealthy';
                    const healthText = client.isHealthy ? '● Healthy' : '● Unhealthy';
                    const actionsCount = client.actions ? client.actions.length : 0;
                    const failCountDisplay = client.failCount > 0 ? '<div class="fail-count">⚠️ 失败次数: ' + client.failCount + '</div>' : '';
                    html += '<div class="client-card">';
                    html += '<div class="client-header">';
                    html += '<div><div class="client-id">' + client.clientId + '</div>';
                    html += '<div class="client-ip">IP: ' + (client.clientIp || 'N/A') + '</div>';
                    html += failCountDisplay + '</div>';
                    html += '<span class="health-badge ' + healthClass + '">' + healthText + '</span>';
                    html += '</div>';
                    html += '<div class="client-actions-section">';
                    html += '<div class="actions-header"><span class="actions-icon">⚡</span> Registered Actions (' + actionsCount + ')</div>';
                    if (client.actions && client.actions.length > 0) {
                        html += '<div class="actions-list">';
                        client.actions.forEach(action => {
                            html += '<span class="action-tag">' + action + '</span>';
                        });
                        html += '</div>';
                    } else {
                        html += '<div class="no-actions">No actions registered</div>';
                    }
                    html += '</div>';
                    html += '<div class="client-footer">';
                    html += '<button class="kick-btn" onclick="kickClient(\'' + group + '\', \'' + client.clientId + '\', this)">🚫 Kick</button>';
                    html += '</div>';
                    html += '</div>';
                });
                html += '</div>';
                html += '</div>';
            });
            html += '</div>';
            document.getElementById('clientsPanel').innerHTML = html;
        }

        async function kickClient(group, clientId, btn) {
            if (!confirm('Are you sure you want to kick client ' + clientId + '?')) {
                return;
            }
            btn.disabled = true;
            btn.textContent = 'Processing...';
            try {
                const res = await fetch('/kick?group=' + encodeURIComponent(group) + '&clientId=' + encodeURIComponent(clientId), {
                    method: 'DELETE'
                });
                const json = await res.json();
                if (json.status === 200) {
                    btn.textContent = 'Kicked';
                    setTimeout(() => fetchData(), 500);
                } else {
                    alert('Failed to kick: ' + json.data);
                    btn.disabled = false;
                    btn.textContent = '🚫 Kick';
                }
            } catch (e) {
                alert('Request failed: ' + e.message);
                btn.disabled = false;
                btn.textContent = '🚫 Kick';
            }
        }

        fetchData();
        setInterval(fetchData, 10000);
    </script>
		</body>
</html>`
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
