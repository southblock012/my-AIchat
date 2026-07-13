package main

import (
	"fmt"
	"my-AIchat/common/mysql"
	"my-AIchat/common/redis"
	"my-AIchat/config"
	"my-AIchat/router"
)

func main() {
	config := config.GetConfig()
	if err := redis.InitRedis(); err != nil {
		panic(err)
	}
	fmt.Println(*config)
	if err := mysql.InitDB(); err != nil {
		panic(err)
	}

	r := router.InitRouter()
	r.Run(":8080")
}
