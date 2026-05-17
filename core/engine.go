package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"context"
	"encoding/json"
	"math/rand"
	"strings"
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

	// Use per-client lock to prevent one client blocking others
	c.wsMu.Lock()
	// Set write deadline to prevent write blocking
	writeDeadline := time.Now().Add(10 * time.Second)
	_ = c.clientWs.SetWriteDeadline(writeDeadline)
	err = c.clientWs.WriteMessage(1, data)
	_ = c.clientWs.SetWriteDeadline(time.Time{})
	c.wsMu.Unlock()
	if err != nil {
		log.Error("当前IP：", clientIp, " 写入数据失败: ", err)
		c.isHealthy = false
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
		// 成功响应，重置失败计数
		c.failCount = 0
		c.isHealthy = true
		resChan <- res
	case <-ctx.Done():
		// 超时，增加失败计数
		c.failCount++
		if c.failCount >= 3 {
			c.isHealthy = false
		}
		utils.LogPrint("当前IP：", clientIp, "超时了。MessageId:", MessageId, " failCount:", c.failCount)
		resChan <- "获取结果超时 timeout"
	}
}

func getRandomClient(group string, clientId string, fuzzy bool) *Clients {
	defer func() {
		if r := recover(); r != nil {
			log.Error("getRandomClient panic recovered: ", r)
		}
	}()

	var client *Clients
	// 不传递clientId时候，从group分组随便拿一个
	if clientId != "" {
		if fuzzy {
			return getFuzzyClient(group, clientId)
		}
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
	return getHealthyClient(group, "")
}

// getFuzzyClient 获取clientId包含指定内容的客户端
func getFuzzyClient(group string, clientId string) *Clients {
	healthyClients := make([]*Clients, 0)
	unhealthyClients := make([]*Clients, 0)

	hlSyncMap.Range(func(_, value interface{}) bool {
		tmpClients, ok := value.(*Clients)
		if !ok {
			log.Warning("类型断言失败：无法将value转换为*Clients")
			return true
		}
		if tmpClients.clientGroup != group || !strings.Contains(tmpClients.clientId, clientId) {
			return true
		}
		if tmpClients.isHealthy && tmpClients.failCount < 3 {
			healthyClients = append(healthyClients, tmpClients)
		} else {
			unhealthyClients = append(unhealthyClients, tmpClients)
		}
		return true
	})

	candidates := healthyClients
	if len(candidates) == 0 {
		candidates = unhealthyClients
	}
	if len(candidates) == 0 {
		return nil
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return candidates[r.Intn(len(candidates))]
}

// getHealthyClient 获取健康的客户端，排除指定的客户端
func getHealthyClient(group string, excludeClientId string) *Clients {
	healthyClients := make([]*Clients, 0)
	unhealthyClients := make([]*Clients, 0)

	//循环读取syncMap 获取group名字的
	hlSyncMap.Range(func(_, value interface{}) bool {
		tmpClients, ok := value.(*Clients)
		if !ok {
			log.Warning("类型断言失败：无法将value转换为*Clients")
			return true
		}
		if tmpClients.clientGroup == group {
			// 排除指定的客户端
			if excludeClientId != "" && tmpClients.clientId == excludeClientId {
				return true
			}
			// 根据健康状态分类
			if tmpClients.isHealthy && tmpClients.failCount < 3 {
				healthyClients = append(healthyClients, tmpClients)
			} else {
				unhealthyClients = append(unhealthyClients, tmpClients)
			}
		}
		return true
	})

	// 优先选择健康的客户端
	candidates := healthyClients
	if len(candidates) == 0 {
		candidates = unhealthyClients // 如果没有健康的，退而求其次用不健康的
	}
	if len(candidates) == 0 {
		return nil
	}
	// 使用随机数发生器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomIndex := r.Intn(len(candidates))
	return candidates[randomIndex]
}
