# AI Chat — 智能对话助手

基于 Go + Vue 的全栈 AI 聊天应用，支持流式对话、长期记忆、多模型切换。

## 技术栈

| 层 | 技术 |
|----|------|
| 后端框架 | Gin (Go) |
| AI 框架 | CloudWeGo Eino |
| 数据库 | MySQL 8.0 (GORM) |
| 向量数据库 | Weaviate（RAG / NL2SQL 混合检索） |
| 缓存 | Redis 7 |
| 消息队列 | RabbitMQ |
| 前端 | Vue 3 + Element Plus + Axios |
| 部署 | Docker Compose |

## 功能特性

- **AI 对话** — 支持流式（SSE）和非流式两种模式
- **多模型切换** — 阿里百炼（OpenAI 兼容）/ Ollama 本地模型
- **会话管理** — 多会话独立管理，保留历史消息
- **长期记忆** — AI 自动从对话中提取用户偏好并跨会话持久化
- **RAG 知识库问答** — 基于内置文档的检索增强生成，自动检索相关片段注入 prompt
- **自然语言查数据库（Text-to-SQL）** — 用日常语言描述需求，自动检索相关表、生成只读 SQL、执行并把结果总结成中文（详见 [nl2sql_README.md](nl2sql_README.md)）
- **上下文窗口** — 保留最近对话历史，Redis 缓存加速
- **用户认证** — JWT 登录/注册，邮箱验证码

## 快速启动（本地开发）

### 前置条件

- Go 1.26+
- Node.js 20+
- MySQL 8.0
- Redis 7
- RabbitMQ（可选，注释掉 `main.go` 中的 `rabbitmq.InitRabbitMQ()` 可跳过）

### 1. 配置

```bash
# 复制配置模板
cp config/exampleconfig.toml config/config.toml
# 编辑 config/config.toml，修改 MySQL/Redis 等连接信息

# 设置 AI 模型环境变量（以阿里百炼为例）
set OPENAI_API_KEY=sk-xxx
set OPENAI_MODEL_NAME=qwen-plus
set OPENAI_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
```

### 2. 启动后端

```bash
# 安装依赖并运行
go mod tidy
go run main.go
```

### 3. 启动前端

```bash
cd vue-frontend
npm install
npm run serve
```

访问 http://localhost:3000

## Docker 部署

```bash
# 1. 配置环境变量
cp deploy/.env.example deploy/.env
# 编辑 deploy/.env，填入 API Key

# 2. 一键启动
docker compose -f deploy/docker-compose.yml up -d --build
```

访问 http://localhost:3000

### 服务端口映射

| 服务 | 内部端口 | 宿主机端口 |
|------|---------|-----------|
| 前端 (Nginx) | 80 | 3000 |
| 后端 | 8080 | 8081 |
| MySQL | 3306 | 3307 |
| Redis | 6379 | 6380 |
| Weaviate | 8080 | 8082 |
| RabbitMQ | 5672 | 5673 |
| RabbitMQ 管理面板 | 15672 | 15673 |

## 项目结构

```
my-AIchat/
├── main.go                      # 入口
├── config/                      # 配置文件
│   ├── config.toml              # 本地配置
│   └── config.go                # 配置读取
├── router/                      # 路由注册
│   ├── router.go                # 主路由
│   ├── user.go                  # 用户相关
│   ├── ai.go                    # AI 聊天相关
│   └── rag.go                   # RAG 知识库问答
├── controller/                  # 控制器（Handler）
│   ├── user/user.go
│   ├── session/session.go
│   └── rag/rag.go
├── service/                     # 业务逻辑
│   ├── user/user.go
│   ├── session/session.go
│   └── rag/rag.go
├── dao/                         # 数据库操作
│   ├── user/user.go
│   ├── session/session.go
│   ├── message/message.go
│   ├── usermemory/usermemory.go
│   └── rag/rag.go               # RAG 分词检索 + knowledge.md 嵌入
├── model/                       # 数据模型
│   ├── user.go
│   ├── session.go
│   ├── message.go
│   └── usermemory.go
├── common/                      # 公共组件
│   ├── redis/                   # Redis 客户端
│   ├── mysql/                   # MySQL + GORM 自动迁移
│   ├── rabbitmq/                # RabbitMQ 消息队列
│   ├── email/                   # 邮件发送
│   ├── aihelper/                # AI 助手核心
│   │   ├── aihelper.go          # 消息管理 + 生成回复
│   │   ├── cache.go             # Redis 上下文窗口缓存
│   │   ├── memory.go            # 长期记忆提取
│   │   ├── manager.go           # AIHelper 管理器
│   │   ├── factory.go           # 模型工厂
│   │   └── external_query.go    # NL2SQL 编排模型 (modelType=4)
│   ├── dbquery/                 # 自然语言查库（Text-to-SQL）
│   │   ├── schema.go            # 读 information_schema + LLM 注解
│   │   ├── catalog.go           # 库表结构缓存 + Weaviate 索引
│   │   ├── retrieval.go         # 混合检索 + 百炼重排 + 关键词兜底
│   │   ├── sqlgen.go            # SQL 生成 + 只读护栏 + 自愈
│   │   └── executor.go          # 只读执行 + 截断 + 结果压文本
│   ├── rag/                     # RAG 检索 + 重排器（被 dbquery 复用）
│   └── code/                    # 错误码定义
├── middleware/jwt/              # JWT 鉴权中间件
├── utils/                       # 工具函数
│   ├── myjwt/jwt.go             # JWT 生成/解析
│   └── utils.go
├── deploy/                      # Docker 部署文件
│   ├── docker-compose.yml
│   ├── backend.Dockerfile
│   ├── frontend.Dockerfile
│   ├── config.docker.toml
│   └── .env.example
└── vue-frontend/                # Vue 前端
    ├── src/
    │   ├── views/               # 页面组件
    │   ├── utils/api.js         # Axios 请求封装
    │   └── router/index.js      # 前端路由
    └── nginx.conf               # Nginx 配置（Docker 部署用）
```

## 核心架构

```
用户 → 浏览器 → Nginx / Vue Dev Server
                    │
               /api/ 反向代理
                    │
               Go 后端 (Gin)
                    │
          ┌────────┼────────┐
          │        │        │
        MySQL    Redis   RabbitMQ
          │        │        │
       持久化    缓存    异步消息
       (消息)   (上下文)  (写入MySQL)
```

### AI 对话流程

```
1. 用户发送消息
2. 从 Redis 加载上下文窗口（最近 20 条消息）
3. 从 MySQL 加载长期记忆（按重要度排序取 top 20）
4. 拼接为 Messages 数组发送给 AI 模型
5. AI 生成回复（流式或非流式）
6. 回复存入 MySQL（通过 RabbitMQ 异步）
7. 更新 Redis 上下文窗口
8. 后台 goroutine 提取长期记忆
```

## API 接口

| 方法 | 路径 | 说明 | 认证 |
|------|------|------|------|
| POST | `/ai-chat/user/login` | 登录 | 否 |
| POST | `/ai-chat/user/register` | 注册 | 否 |
| POST | `/ai-chat/user/captcha` | 发送验证码 | 否 |
| GET | `/ai-chat/chat/sessions` | 获取会话列表 | JWT |
| POST | `/ai-chat/chat/send` | 发送消息（非流式） | JWT |
| POST | `/ai-chat/chat/send-stream` | 发送消息（流式 SSE） | JWT |
| POST | `/ai-chat/chat/send-new-session` | 新建会话并发送 | JWT |
| POST | `/ai-chat/chat/send-stream-new-session` | 新建会话 + 流式 | JWT |
| POST | `/ai-chat/chat/history` | 获取会话历史 | JWT |
| POST | `/ai-chat/rag/chat` | RAG 知识库问答 | JWT |

## 长期记忆系统

AI 在每次对话后自动提取关键信息，跨会话持久化：

- **事实**（fact）：用户的个人信息、背景
- **偏好**（preference）：用户喜欢的回答风格
- **项目**（project）：用户正在做的事情
- **结论**（conclusion）：达成的共识
- **禁忌**（avoid）：用户不喜欢的回答方式

记忆按重要度（1-5）和访问频率排序，每次对话最多注入 top 20 条到 system prompt 中。

## RAG 知识库

基于内置文档的检索增强生成（RAG），不经过 AIHelper，独立路由 `/ai-chat/rag/chat`。

### 流程

```
1. 用户提问
2. 中文 bigram + 英文关键词分词
3. 全量扫描 rag_chunks 表按关键词命中数打分
4. 取 top-4 相关片段
5. 注入 system prompt 作为【资料】
6. 调用 AI 模型生成回答（仅基于资料，不编造）
```

### 知识来源

- 文档嵌入在 `dao/rag/knowledge.md`（Go embed）
- 首次调用时自动建表并灌入库
- 每段按空行分割，过滤短片段（< 10 字）
- 零外部依赖，不使用向量数据库

---

## 自然语言查数据库（Text-to-SQL）

> 在 RAG 的基础上新增 **Text-to-SQL** 能力：在聊天界面选择「自然语言查数据库」（modelType=4），
> 用日常语言描述需求（如「最近注册的 10 个用户」），系统会自动检索相关表 → 生成只读 SQL → 执行 → 把结果总结成中文。
> 后端完全复用项目既有的 **Weaviate + 百炼 embedding + cross-encoder 重排 + 混合检索** 设施，
> 仅在 `common/dbquery` 包内把「文档」换成「库表结构」做向量化与检索；前端只在模型下拉框加了一个选项。

### 数据流

1. 前端透传 `modelType=4`，`service` / `controller` 零改动；
2. `aihelper.Manager` 按 `modelType` 创建 `ExternalQueryModel`（复用三层会话 map）；
3. 编排 `dbquery` 子管道：混合检索相关表 → 拼表结构 prompt → LLM 生成 SQL → 只读护栏校验 → 带超时执行（截断 `maxRows`）→ 失败自愈一次 → LLM 总结成口语化中文；
4. 复用 `/ai-chat/chat/send`（同步返回总结）与 `/ai-chat/chat/send-stream`（流式先推 SQL 代码块再推总结）接口。

### 安全护栏（三层）

- **连接层**：外部库建议使用**只读账号** DSN；
- **代码层**：只读白名单（仅 `SELECT / SHOW / DESCRIBE / WITH / EXPLAIN`）+ 拒绝多语句 + 强制 `LIMIT` + 查询超时；
- **结果层**：执行结果截断到 `maxRows` 行，结果文本二次字符预算截断。

### 配置

`config.toml` 的 `[externalDBConfig]` 段：

```toml
[externalDBConfig]
dsn                 = "root:123456@tcp(127.0.0.1:3306)/"   # 外部库连接串（不含库名，库名走下方 schema）
schema              = "aichat"                              # 要查的库名（TABLE_SCHEMA），必填
topK                = 5                                     # 检索阶段返回的最大相关表数
maxRows             = 100                                   # 单条查询最多返回行数
queryTimeoutSeconds = 10                                    # 单条查询超时秒数
```

同时需要 `EMBEDDING_API_KEY`（百炼/DashScope 密钥，与 RAG 共用同一变量）。
embedding / rerank / 混合权重均来自 `config.RagModelConfig`，**无需为 NL2SQL 单独配置**。

> 完整的改动文件清单、各 Phase 详解、运行前提与验证步骤见 [nl2sql_README.md](nl2sql_README.md)。
