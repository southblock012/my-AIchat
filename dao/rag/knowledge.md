# my-AIchat 项目知识库

项目是一个基于 Go 和 Vue 的 AI 聊天应用。后端使用分层架构，包含 router、controller、service、dao、model 五层，并接入 DeepSeek 大语言模型提供对话能力。

后端核心流程是：用户发消息后经过 JWT 鉴权，由 AIHelper 调用大语言模型生成回复；消息通过 RabbitMQ 异步持久化到 MySQL，主回复流程不被数据库写入阻塞。

Docker 部署使用 docker-compose 编排 mysql、redis、rabbitmq、backend、frontend 五个服务。backend 的配置文件通过 bind mount 挂载进容器，mysql 和 redis 的数据通过 named volume 持久化。

配置管理采用配置金字塔原则：默认值低于配置文件，配置文件低于环境变量，环境变量低于命令行参数。敏感信息如 API Key 放在 .env 文件中，通过环境变量注入容器，不写进镜像，避免密钥泄露。

挂载卷分为 bind mount 和 named volume 两种。bind mount 用于挂载宿主机配置文件（如 config.toml），named volume 用于持久化数据库数据。数据卷内容存放在 Docker 内部存储区，不会污染项目目录。

长期记忆模块把用户的事实和偏好抽取到 user_memory 表，支持冲突检测与遗忘机制。新记忆权重较低时先标记为待确认，多次出现才生效，避免错误记忆被固化成用户事实。

RabbitMQ 在项目中负责消息异步持久化。backend 发布消息到队列，消费者订阅后将消息写入 MySQL。这样即使数据库写入慢或失败，也不影响用户收到 AI 回复。

前端使用 Vue 框架。登录后 token 存储在浏览器 localStorage 中，每次请求通过 Authorization 头携带 Bearer token 完成 JWT 鉴权。token 只包含用户名和 id，由后端签名验证。

nginx 在 frontend 中作为反向代理。处理 SSE 流式响应时需要设置 proxy_http_version 1.1、Connection 为空、proxy_buffering off，否则流式传输会被缓冲导致前端收不到逐字输出。

多轮对话的上下文来自 Redis 滑动窗口 session:{id}:ctx，保存最近 20 条消息，TTL 随活跃续期；窗口未命中时从 MySQL 回灌历史。这样重启或多实例部署后上下文不丢失，也控制了发送给模型的 token 数量。
