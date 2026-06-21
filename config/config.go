package config

import (
	"github.com/BurntSushi/toml"

	"log"
)

type MainConfig struct {
	AppName string `toml:"appName"`
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
}

type MysqlConfig struct {
	MysqlHost         string `toml:"host"`
	MysqlPort         int    `toml:"port"`
	MysqlUser         string `toml:"username"`
	MysqlPassword     string `toml:"password"`
	MysqlDatabaseName string `toml:"databaseName"`
	MysqlCharset      string `toml:"charset"`
}

// Config 配置结构体(包含所有配置项)
type Config struct {
	MainConfig  `toml:"mainConfig"`
	MysqlConfig `toml:"mysqlConfig"`
}

var config *Config

// InitConfig 初始化项目配置
func InitConfig() error {
	// 设置配置文件路径（相对于 main.go 所在的目录）
	if _, err := toml.DecodeFile("config/config.toml", config); err != nil {
		log.Fatal(err.Error())
		return err
	}
	return nil
}

func GetConfig() *Config {
	if config == nil {
		config = new(Config)
		_ = InitConfig()
	}
	return config
}
