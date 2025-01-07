package core

import (
	"JsRpc/config"
	"JsRpc/utils"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"time"
)

// GQueryFunc 发送请求到客户端
func (c *Clients) GQueryFunc(funcName string, param string, resChan chan<- string) {
	if c.actionData[funcName] == nil {
		c.actionData[funcName] = make(map[string]chan string)
	}
	MessageId := ""
	gm.Lock()
	for {
		MessageId = utils.GetUUID()
		// 先判断action是否需要初始化
		if c.actionData[funcName][MessageId] == nil {
			c.actionData[funcName][MessageId] = make(chan string, 1) //此次action初始化1个消息
			//只有不存在的MessageId才会继续，
			break
		} else {
			utils.LogPrint("存在的消息id,跳过")
		}
	}
	gm.Unlock()
	WriteData := Message{Param: param, MessageId: MessageId, Action: funcName}
	data, _ := json.Marshal(WriteData)
	clientWs := c.clientWs
	err := clientWs.WriteMessage(1, data)
	if err != nil {
		log.Error(err, "写入数据失败")
		resChan <- "rpc发送数据失败"
	}
	select {
	case res := <-c.actionData[funcName][MessageId]:
		resChan <- res
	case <-time.After(time.Duration(config.DefaultTimeout) * time.Second):
		utils.LogPrint(MessageId + "超时了")
		resChan <- "黑脸怪：timeout"
	}
	// 清理资源
	gm.Lock()
	delete(c.actionData[funcName], MessageId)
	gm.Unlock()

	close(resChan)
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
