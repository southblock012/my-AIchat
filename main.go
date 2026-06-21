package main

import (
	"fmt"
	"my-AIchat/config"
	"my-AIchat/router"
)

func main() {
	config := config.GetConfig()
	fmt.Println(*config)
	r := router.InitRouter()
	r.Run(":8080")
}
