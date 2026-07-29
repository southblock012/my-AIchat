package weaviate

import (
	"github.com/weaviate/weaviate-go-client/v5/weaviate"
)

var WeaviateClient *weaviate.Client

func InitWeaviate() *weaviate.Client {
	client, err := weaviate.NewClient(weaviate.Config{
		Scheme: "http",
		Host:   "localhost:8080",
	})
	if err != nil {
		return nil
	}
	WeaviateClient = client
	return client
}
