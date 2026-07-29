package weaviate

import (
	"log"

	"my-AIchat/config"

	wv "github.com/weaviate/weaviate-go-client/v5/weaviate"
)

// WeaviateClient 是全局唯一的 Weaviate 客户端，由 InitWeaviate 在启动时初始化。
// rag 包（common/rag）直接复用它，避免每次请求都新建连接。
var WeaviateClient *wv.Client

// InitWeaviate 构造 Weaviate 客户端。
// host 从配置 ragModelConfig.weaviateHost 读取：
//   - 本机直跑后端：localhost:8082（宿主机映射端口）
//   - Docker 内部：weaviate:8080（服务名 + 容器端口）
func InitWeaviate() *wv.Client {
	host := config.GetConfig().WeaviateConfig.WeaviateHost

	client, err := wv.NewClient(wv.Config{
		Scheme: "http",
		Host:   host,
	})
	if err != nil {
		log.Printf("[InitWeaviate] failed to create client (host=%s): %v", host, err)
		return nil
	}
	WeaviateClient = client
	log.Printf("[InitWeaviate] client created (host=%s)", host)
	return client
}
