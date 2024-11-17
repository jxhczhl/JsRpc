package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// GQueryFunc 发送请求到客户端
func (c *Clients) GQueryFunc(funcName string, param string, resChan chan<- string) {
	MessageId := utils.GetUUID()
	WriteData := Message{Param: param, MessageId: MessageId, Action: funcName}
	data, _ := json.Marshal(WriteData)
	clientWs := c.clientWs
	// 先判断action是否需要初始化
	if c.actionData[funcName] == nil {
		c.actionData[funcName] = make(map[string]chan string)
	}
	if c.actionData[funcName][MessageId] == nil {
		c.actionData[funcName][MessageId] = make(chan string, 1) //此次action初始化1个消息
	}
	gm.Lock()
	err := clientWs.WriteMessage(1, data)
	gm.Unlock()
	if err != nil {
		fmt.Println(err, "写入数据失败")
	}
	resultFlag := false
	for i := 0; i < config.DefaultTimeout*10; i++ {
		if len(c.actionData[funcName][MessageId]) > 0 {
			res := <-c.actionData[funcName][MessageId]
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
		delete(c.actionData[funcName], MessageId)
	}()
}

func getRandomClient(group string, clientId string) *Clients {
	var client *Clients
	// 不传递clientId时候，从group分组随便拿一个
	if clientId != "" {
		clientName, ok := hlSyncMap.Load(group + "->" + clientId)
		if ok == false {
			return nil
		}
		client, _ = clientName.(*Clients)
		return client
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
		return nil
	}
	// 使用随机数发生器
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomIndex := r.Intn(len(groupClients))
	client = groupClients[randomIndex]
	return client

}
