package main

import (
	"fmt"
	"my-AIchat/common/mysql"
	"my-AIchat/config"
	"my-AIchat/router"
)

func main() {
	config := config.GetConfig()
	fmt.Println(*config)
	if err := mysql.InitDB(); err != nil {
		panic(err)
	}
	r := router.InitRouter()
	r.Run(":8080")
}
