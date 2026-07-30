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

type JwtConfig struct {
	ExpireDuration int    `toml:"expire_duration"`
	Issuer         string `toml:"issuer"`
	Subject        string `toml:"subject"`
	Key            string `toml:"key"`
}

type EmailConfig struct {
	AuthCode string `toml:"authcode"`
	Email    string `toml:"email"`
}

type RedisConfig struct {
	RedisHost     string `toml:"host"`
	RedisPort     int    `toml:"port"`
	RedisPassword string `toml:"password"`
	RedisDb       int    `toml:"db"`
}

type RabbitmqConfig struct {
	RabbitmqHost     string `toml:"host"`
	RabbitmqPort     int    `toml:"port"`
	RabbitmqUsername string `toml:"username"`
	RabbitmqPassword string `toml:"password"`
	RabbitmqVhost    string `toml:"vhost"`
}

type RedisKeyConfig struct {
	CaptchaPrefix   string
	IndexName       string
	IndexNamePrefix string
}

type RagModelConfig struct {
	RagEmbeddingModel string `toml:"embeddingModel"`
	RagChatModelName  string `toml:"chatModelName"`
	RagDocDir         string `toml:"docDir"`
	RagBaseUrl        string `toml:"baseUrl"`
	RagDimension      int    `toml:"dimension"`
	// 增强检索配置（混合检索 + 重排）
	RagHybridAlpha float64 `toml:"hybridAlpha"` // 混合检索中向量 vs 关键词权重：0=纯关键词(BM25)，1=纯向量，默认 0.5
	RagRetrieveK   int     `toml:"retrieveK"`   // 混合检索候选数（重排前的召回量），默认 20
	RagRerankTopK  int     `toml:"rerankTopK"`  // 重排后最终返回条数，默认 5
	RagRerankModel string  `toml:"rerankModel"` // 重排模型；非空时启用 eino 重排，默认 gte-rerank
	RagRerankBaseUrl string `toml:"rerankBaseUrl"` // 重排服务地址；留空用百炼默认地址
}

type WeaviateConfig struct {
	WeaviateHost string `toml:"host"`
}

// Config 配置结构体(包含所有配置项)
type Config struct {
	MainConfig     `toml:"mainConfig"`
	MysqlConfig    `toml:"mysqlConfig"`
	JwtConfig      `toml:"jwtConfig"`
	EmailConfig    `toml:"emailConfig"`
	RedisConfig    `toml:"redisConfig"`
	RabbitmqConfig `toml:"rabbitmqConfig"`
	RagModelConfig `toml:"ragModelConfig"`
	WeaviateConfig `toml:"weaviateConfig"`
}

var DefaultRedisKeyConfig = RedisKeyConfig{
	CaptchaPrefix:   "captcha:%s",
	IndexName:       "rag_docs:%s:idx",
	IndexNamePrefix: "rag_docs:%s:",
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
