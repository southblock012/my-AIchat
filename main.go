package main

import (
	"context"
	"fmt"
	"log"
	"my-AIchat/common/aihelper"
	"my-AIchat/common/dbquery"
	"my-AIchat/common/mysql"
	"my-AIchat/common/rabbitmq"
	"my-AIchat/common/redis"
	"my-AIchat/common/weaviate"
	"my-AIchat/config"
	"my-AIchat/dao/message"
	"my-AIchat/router"
)

func StartServer(addr string, port int) error {
	r := router.InitRouter()
	//服务器静态资源路径映射关系，这里目前不需要
	// r.Static(config.GetConfig().HttpFilePath, config.GetConfig().MusicFilePath)
	return r.Run(fmt.Sprintf("%s:%d", addr, port))
}

// 从数据库加载消息并初始化 AIHelperManager
func readDataFromDB() error {
	manager := aihelper.GetGlobalManager()
	// 从数据库读取所有消息
	msgs, err := message.GetAllMessages()
	if err != nil {
		return err
	}
	// 遍历数据库消息
	for i := range msgs {
		m := &msgs[i]
		//默认openai模型
		modelType := "1"
		config := make(map[string]interface{})

		// 创建对应的 AIHelper
		helper, err := manager.GetOrCreateAIHelper(m.UserName, m.SessionID, modelType, config)
		if err != nil {
			log.Printf("[readDataFromDB] failed to create helper for user=%s session=%s: %v", m.UserName, m.SessionID, err)
			continue
		}
		log.Println("readDataFromDB init:  ", helper.SessionID)
		// 添加消息到内存中(不开启存储功能)
		helper.AddMessage(m.Content, m.UserName, m.IsUser, false)
	}

	log.Println("AIHelperManager init success ")
	return nil
}

func main() {
	conf := config.GetConfig()
	host := conf.MainConfig.Host
	port := conf.MainConfig.Port

	//初始化mysql
	if err := mysql.InitDB(); err != nil {
		log.Println("InitMysql error , " + err.Error())
		return
	}
	//初始化外部数据库（自然语言查库目标库）：独立连接，失败不阻断主流程
	if err := mysql.InitExternalDB(); err != nil {
		log.Println("[externalDB] init failed (查库功能不可用): " + err.Error())
	} else if mysql.ExternalDB != nil {
		// best-effort 预热 schema 缓存（含 LLM 含义补全，只发生一次），放后台避免拖慢启动
		go func() {
			if err := dbquery.InitCatalog(context.Background()); err != nil {
				log.Printf("[dbquery] catalog 预热失败(后续查询时按需重建): %v", err)
			}
		}()
	}
	//初始化AIHelperManager
	readDataFromDB()

	//初始化weaviate
	weaviate.InitWeaviate()
	log.Println("weaviate init success  ")

	//初始化redis
	redis.InitRedis()
	log.Println("redis init success  ")

	//初始化rabbitmq
	rabbitmq.InitRabbitMQ()
	log.Println("rabbitmq init success  ")
	err := StartServer(host, port) // 启动 HTTP 服务
	if err != nil {
		panic(err)
	}
}
