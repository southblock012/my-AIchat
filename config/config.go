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
	RagHybridAlpha   float64 `toml:"hybridAlpha"`   // 混合检索中向量 vs 关键词权重：0=纯关键词(BM25)，1=纯向量，默认 0.5
	RagRetrieveK     int     `toml:"retrieveK"`     // 混合检索候选数（重排前的召回量），默认 20
	RagRerankTopK    int     `toml:"rerankTopK"`    // 重排后最终返回条数，默认 5
	RagRerankModel   string  `toml:"rerankModel"`   // 重排模型；非空时启用 eino 重排，默认 gte-rerank
	RagRerankBaseUrl string  `toml:"rerankBaseUrl"` // 重排服务地址；留空用百炼默认地址
}

type WeaviateConfig struct {
	WeaviateHost string `toml:"host"`
}

// ExternalDBConfig 自然语言查库用的「外部数据库」连接配置。
// 与目标库（本项目业务库）完全解耦：独立 DSN、独立连接池、可选 LLM 含义补全。
// 强烈建议使用只读账号的 DSN，从连接层杜绝写入。
type ExternalDBConfig struct {
	Dsn          string `toml:"dsn"`          // 完整 DSN，如 root:pass@tcp(host:3306)/ ；末尾可不带库名，库名用 Schema 指定
	Schema       string `toml:"schema"`       // 要查询的库名（schema）；显式指定，不依赖 DSN 默认库/DATABASE()
	MaxIdleConns int    `toml:"maxIdleConns"` // 连接池空闲连接数，0 则用默认 5
	MaxOpenConns int    `toml:"maxOpenConns"` // 连接池最大连接数，0 则用默认 20
	Annotate     bool   `toml:"annotate"`     // 是否用 LLM 补全表/字段中文含义（仅当 COMMENT 缺失时）
	TopK               int `toml:"topK"`               // 相关表召回数（喂给 Text-to-SQL 的表数量），0 则用默认 5
	MaxRows            int `toml:"maxRows"`            // 单次查询结果最多返回行数（防大结果集），0 则用默认 100
	QueryTimeoutSeconds int `toml:"queryTimeoutSeconds"` // SQL 执行超时（秒），0 则用默认 10
}

// Config 配置结构体(包含所有配置项)
type Config struct {
	MainConfig       `toml:"mainConfig"`
	MysqlConfig      `toml:"mysqlConfig"`
	JwtConfig        `toml:"jwtConfig"`
	EmailConfig      `toml:"emailConfig"`
	RedisConfig      `toml:"redisConfig"`
	RabbitmqConfig   `toml:"rabbitmqConfig"`
	RagModelConfig   `toml:"ragModelConfig"`
	WeaviateConfig   `toml:"weaviateConfig"`
	ExternalDBConfig `toml:"externalDBConfig"`
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
