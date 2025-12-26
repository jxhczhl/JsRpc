package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"context"
	"encoding/json"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
)

// GQueryFunc 发送请求到客户端
func (c *Clients) GQueryFunc(funcName string, param string, resChan chan<- string, clientIp string) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("GQueryFunc panic recovered: ", r)
			// 尝试安全地发送错误消息
			defer func() {
				if r2 := recover(); r2 != nil {
					log.Error("发送错误消息到channel也失败了: ", r2)
				}
			}()
			resChan <- "内部错误"
			close(resChan)
		}
	}()

	if c.actionData[funcName] == nil {
		rwMu.Lock()
		c.actionData[funcName] = make(map[string]chan string)
		rwMu.Unlock()
	}
	var MessageId string
	for {
		MessageId = utils.GetUUID()
		if c.readFromMap(funcName, MessageId) == nil {
			rwMu.Lock()
			c.actionData[funcName][MessageId] = make(chan string, 1)
			rwMu.Unlock()
			break
		}
		utils.LogPrint("存在的消息id,跳过")
	}
	// 确保资源释放
	defer func() {
		rwMu.Lock()
		if c.actionData[funcName] != nil {
			if ch, exists := c.actionData[funcName][MessageId]; exists {
				delete(c.actionData[funcName], MessageId)
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Debug("关闭已关闭的channel: ", r)
						}
					}()
					close(ch)
				}()
			}
		}
		rwMu.Unlock()
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Debug("关闭resChan失败: ", r)
				}
			}()
			close(resChan)
		}()
	}()

	// 构造消息并发送
	WriteData := Message{Param: param, MessageId: MessageId, Action: funcName}
	data, err := json.Marshal(WriteData)
	if err != nil {
		log.Error("当前IP：", clientIp, " JSON序列化失败: ", err)
		resChan <- "JSON序列化失败"
		return
	}

	rwMu.Lock()
	err = c.clientWs.WriteMessage(1, data)
	rwMu.Unlock()
	if err != nil {
		log.Error("当前IP：", clientIp, " 写入数据失败: ", err)
		resChan <- "rpc发送数据失败"
		return
	}
	// 使用 context 控制超时
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.DefaultTimeout)*time.Second)
	defer cancel()
	resultChan := c.readFromMap(funcName, MessageId)
	if resultChan == nil {
		resChan <- "消息ID对应的管道不存在"
		return
	}
	select {
	case res := <-resultChan:
		resChan <- res
	case <-ctx.Done():
		utils.LogPrint("当前IP：", clientIp, "超时了。", MessageId)
		resChan <- "获取结果超时 timeout"
	}
}

func getRandomClient(group string, clientId string) *Clients {
	defer func() {
		if r := recover(); r != nil {
			log.Error("getRandomClient panic recovered: ", r)
		}
	}()

	var client *Clients
	// 不传递clientId时候，从group分组随便拿一个
	if clientId != "" {
		clientName, ok := hlSyncMap.Load(group + "->" + clientId)
		if !ok {
			return nil
		}
		client, ok = clientName.(*Clients)
		if !ok {
			log.Error("类型断言失败：无法将clientName转换为*Clients")
			return nil
		}
		return client
	}
	groupClients := make([]*Clients, 0)
	//循环读取syncMap 获取group名字的
	hlSyncMap.Range(func(_, value interface{}) bool {
		tmpClients, ok := value.(*Clients)
		if !ok {
			log.Warning("类型断言失败：无法将value转换为*Clients")
			return true
		}
		if tmpClients.clientGroup == group {
			groupClients = append(groupClients, tmpClients)
		}
		return true
	})
	if len(groupClients) == 0 {
		return nil
	}
	// 使用随机数发生器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomIndex := r.Intn(len(groupClients))
	client = groupClients[randomIndex]
	return client

}
