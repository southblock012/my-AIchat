// Package test 存放 RAG 检索增强的离线评估用例。
//
// 本测试对比三种检索策略在同一份黄金问答集上的召回率：
//   - 朴素(前)：纯向量 ANN（NearVector cosine Top5）
//   - 混合(中)：Weaviate 原生 hybrid（BM25 + 向量融合，无重排）
//   - 混合+重排(后)：hybrid 候选 → eino cross-encoder 精排
// 指标为 recall@1 / recall@3 / recall@5 与 MRR，并额外输出「逐条排名变化」以隔离 rerank 的贡献。
//
// 运行依赖（缺失则自动 skip，不会让 CI 红）：
//   - 本地 Weaviate：默认 http://localhost:8082，可用环境变量 WEAVIATE_HOST 覆盖；
//   - 百炼 embedding / rerank 密钥：环境变量 EMBEDDING_API_KEY；
//   - 配置文件 config/config.toml（测试会自动回退到仓库根目录加载）。
//
// 运行方式（在仓库根目录执行）：
//
//	EMBEDDING_API_KEY=sk-xxx go test ./test -run TestRAGRecallBeforeAfter -v
package test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"my-AIchat/common/rag"
	"my-AIchat/config"

	"github.com/cloudwego/eino/schema"
)

// goldenDoc 一份合成的「产品手册」知识库。
//
// 设计要点（让测试能真实反映 rerank 价值，而非玩具集）：
//  1. 体量足够大：约 20+ 章节，切块后产生 15~25 个 chunk，使 retrieveK=20 真正裁剪候选；
//  2. 每个主题都配有「硬负例」邻居章节——与目标章节语义高度相近（共享大量同义词），
//     但答案不同。例如「脚本启动(start.sh)」旁有「容器启动(docker run)」「系统服务启动(systemctl)」，
//     向量检索容易把硬负例排到答案前面，而 cross-encoder rerank 看 (query, chunk) 配对文本能识别真答案；
//  3. 每个章节内嵌一个可被关键词唯一锁定的 needle，便于客观判定命中。
const goldenDoc = `# 脚本启动方式
本服务通过 start.sh 脚本启动，启动后会监听默认端口 PORT=8000。执行 chmod +x start.sh 赋予执行权限后，运行 ./start.sh 即可在前台拉起服务。若需要后台常驻，可配合 nohup ./start.sh & 使用，并将标准输出重定向到 nohup.out 便于排查。启动前请确保配置文件 config.yaml 已正确填写，尤其是数据库连接信息与 API 密钥。启动成功后可通过 curl http://localhost:8000/ 验证首页是否可访问。若启动即退出，请优先查看日志中的 panic 堆栈，常见原因是配置缺失或端口已被占用。该方式适合单机调试与快速验证功能。

# 容器启动方式
使用 Docker 部署时，通过 docker run 命令启动服务，并用 -p 8000:8000 将容器端口映射到宿主机的 8000 端口。推荐编写 docker-compose.yml 声明服务，使用 docker compose up -d 在后台拉起。容器启动前需通过环境变量注入配置，例如 -e PORT=8000 -e DATABASE_URL=xxx，而非挂载配置文件，以避免密钥写入镜像层。建议在 compose 文件中配置 restart: unless-stopped 实现异常自动重启。容器场景下服务监听 0.0.0.0:8000，对外暴露端口由映射决定。若容器反复重启，请检查健康检查端点 /healthz 是否返回 200。该方式适合标准化交付与多环境一致性部署。

# 系统服务启动方式
在服务器上可将服务注册为 systemd 单元，编写 myaichat.service 文件放入 /etc/systemd/system/，执行 systemctl daemon-reload 后通过 systemctl start myaichat 启动，并设置 systemctl enable myaichat 实现开机自启。服务单元中应声明 User、WorkingDirectory 与 Environment 环境变量（如 PORT=8000），并用 Restart=on-failure 控制崩溃重启策略。通过 journalctl -u myaichat 可查看运行日志。该方式适合生产环境的进程托管与统一日志收集，避免 nohup 进程随会话退出而丢失。

# 应用计数接口
调用 /api/count 接口可以返回系统内已安装应用的总数，返回格式为 JSON，字段 total 表示数量。该接口只读，不依赖任何查询参数，适合做健康探针与监控大盘的采集源。在高并发场景下建议加一层短期缓存，避免每次请求都扫描数据库造成压力。接口默认开启鉴权，需要在 Header 中携带合法 Token。与运行指标接口不同，它只关心「应用个数」这一单一整数，不做任何聚合统计。若返回 0，通常说明采集任务尚未完成首轮同步。

# 运行指标接口
调用 /api/metrics 接口可以返回服务的运行指标，包括 QPS、内存占用、GC 次数与采集耗时等分位数数据，格式同样为 JSON。该接口面向运维监控，配合 Prometheus 拉取后可在 Grafana 中绘制趋势图。与 /api/count 仅返回总数不同，/api/metrics 提供的是多维度的性能剖面，用于容量评估与异常定位。接口也需鉴权，且建议限制其暴露范围，避免内部指标外泄。若指标长时间不更新，应检查采集协程是否阻塞。

# 应用列表接口
调用 /api/list 接口可以分页返回已安装应用列表，支持 limit 与 offset 参数，单页默认 20 条。返回字段包含应用名、版本、来源平台与安装时间。该接口支持按平台过滤，例如 ?platform=windows 仅返回 Windows 端应用。对于超长列表，建议配合游标分页以降低数据库压力，而非深翻页。与 /api/count 只给数字不同，/api/list 返回的是结构化记录集合。同样需要鉴权 Token，且对大结果集应启用字段裁剪。

# 全文搜索接口
调用 /api/search 接口可以按关键字搜索已安装应用，参数为 q 与可选的 platform 过滤，返回匹配应用列表。该接口在应用名与厂商字段上做模糊匹配，适合前端搜索框的自动补全。与 /api/list 的整齐分页不同，/api/search 的排序依据相关度得分而非安装时间。搜索同样需要鉴权，且对高频词建议加结果缓存。若搜索无结果，可能是索引尚未构建或关键字过于冷僻。该接口与 /api/list 共用底层数据表，仅查询方式不同。

# Windows 注册表采集
在 Windows 系统上，服务通过读取注册表 Uninstall 键获取已安装应用信息，遍历 HKLM 与 HKCU 下的卸载项，收集应用名与版本号。对于 Microsoft Store 应用，还需查询 Windows Management Instrumentation 中的 Win32_Product 类补充信息。由于注册表存在 32 位与 64 位两套视图，服务会同时枚举以保证不遗漏。解析时应注意处理带大括号的 GUID 显示名称，并跳过系统组件。该采集路径对权限敏感，需在具备读取注册表权限的账户下运行。

# Windows WMI 采集
在 Windows 系统上，服务也可通过 WMI 的 Win32_Product 类枚举已安装产品，适合需要统一查询接口的场景。与注册表 Uninstall 键相比，Win32_Product 返回的是更规范的安装包清单，但查询开销更大，且在某些系统上会触发不必要的安装修复行为，因此仅作为注册表采集的补充。采集时过滤掉 Name 为空或 Publisher 为系统的记录。该路径同样需要管理员权限，且与注册表路径互为候选方案。若两者结果不一致，以注册表为准。

# macOS 应用采集
在 macOS 系统上，服务通过 system_profiler SPApplicationsDataType 命令读取已安装应用信息，并解析 /Applications 目录下的 .app 包。对于 Mac App Store 安装的应用，还需读取其收据信息以确认购买状态与版本。由于 macOS 沙盒限制，部分系统应用可能无法被完整枚举。建议在首次运行时请求辅助功能权限以提升覆盖率。与 Windows 的双路径不同，macOS 主要依赖 system_profiler 单一来源，必要时回退到 Spotlight 元数据。

# Linux 包管理器采集
在 Linux 系统上，服务通过 dpkg 或 rpm 包管理器获取已安装应用列表，支持 Debian 与 RedHat 系发行版。对于通过源码编译安装的程序，包管理器无法感知，此时会回退到遍历常见 bin 目录进行启发式识别。容器环境下建议挂载宿主机的包数据库以获得准确结果。Arch 系发行版使用 pacman，可做后续扩展。与 macOS 的 system_profiler 不同，Linux 的采集分散在多个包管理器后端，需按发行版动态选择命令。解析时注意去重同名不同版本条目。

# 端口配置
通过环境变量 PORT 可以修改服务监听的端口，默认值 8000；修改后需重启服务生效。若端口被占用，服务会启动失败并在日志中提示 bind: address already in use。在容器部署时，PORT 还需与镜像暴露端口及反向代理配置保持一致。建议使用进程管理器确保端口释放后再重启，避免 TIME_WAIT 状态导致绑定失败。该配置仅影响监听地址，与数据库连接无关。若需监听所有网卡，可设 0.0.0.0:PORT。注意不可与数据库端口混淆。

# 数据库连接配置
通过环境变量 DATABASE_URL 配置服务连接的后端数据库，格式为 postgres://user:pass@host:5432/dbname。修改后需重启服务并建立连接池。若连接失败，服务会在启动阶段报 connection refused 并退出。与监听端口 PORT 相互独立，二者不应混为一谈。建议在连接串中启用 sslmode=require 以加密传输。连接池大小通过 DB_MAX_CONN 控制，默认值应小于数据库服务端的最大连接数。连接超时通过 DB_TIMEOUT 设定，避免雪崩。

# 日志配置
通过环境变量 LOG_LEVEL 控制日志级别，可选 debug、info、warn、error，默认 info。日志默认输出到标准输出，容器环境由采集器统一收集。与端口、数据库配置不同，日志配置不影响服务功能，仅影响可观测性。建议在排查问题时临时调高到 debug，定位后回落 info 以避免日志膨胀。日志轮转由外部工具处理，服务本身不负责切割。敏感字段（如密钥）在落库前会做脱敏，日志中亦不打印明文 Token。

# 端口占用排查
若服务启动失败，提示 bind: address already in use，说明目标端口已被其他进程占用。可通过 lsof -i:8000 或 netstat -tlnp 找到占用进程并终止，或改用其他 PORT 启动。常见原因是上一次实例未正常退出留下僵尸进程。与数据库连接失败不同，此类问题仅与网络端口相关，不涉及凭证。若频繁出现，建议检查进程管理器是否重复拉起了多个实例。定位后重启即可恢复监听。

# 数据库连接超时排查
若服务启动或运行中报 connection timeout，说明无法在 DB_TIMEOUT 内连上后端数据库。应依次检查数据库进程是否存活、DATABASE_URL 中的 host 与端口是否正确、网络策略是否放行，以及连接池是否耗尽。与端口占用不同，该问题根因在数据库侧而非本机端口。可通过 telnet host 5432 验证连通性。若数据库负载高，应调大 DB_MAX_CONN 或优化慢查询。收集数据库慢日志有助于定位。

# 数据同步机制
服务每分钟执行一次数据同步，将各平台采集到的应用信息汇总写入本地数据库。同步过程为增量更新，仅写入较上次发生变化的记录，并基于应用标识做 upsert。若某平台采集失败，不会影响其他平台，下一次周期会重试。可通过管理后台手动触发全量同步以修复不一致。与实时接口不同，同步是后台批量任务，其延迟决定了 /api/list 等接口数据的鲜度。同步状态可通过 /api/sync/status 查询。

# 安全与鉴权
服务所有写操作均需携带鉴权 Token，Token 采用短期有效期并支持主动吊销。读接口默认也开启鉴权，仅健康检查端点例外。敏感字段在落库前会做脱敏处理。建议配合反向代理开启 HTTPS 以防止中间人窃听。与端口、数据库等基础配置不同，鉴权是安全边界，缺失会导致数据泄露。Token 通过 Authorization: Bearer 头传递，过期后客户端需重新获取。建议定期轮换签名密钥。

# 关键字检索加速
当已安装应用数量超过一万时，建议对应用名建立倒排索引以加速关键字检索，尤其是 /api/search 的模糊匹配。同时可对热点查询结果做短期缓存。对于多实例部署，推荐使用共享缓存而不是各自本地缓存，避免结果不一致。定期清理历史同步日志也有助于保持数据库体积稳定。与连接池优化不同，倒排索引针对的是查询侧的检索效率，是搜索性能的关键路径。

# 连接池优化
在高并发场景下，应调大数据库连接池上限 DB_MAX_CONN 以避免请求在获取连接时排队。同时设置合理的 DB_TIMEOUT 防止慢连接拖垮整体吞吐。与倒排索引优化查询侧不同，连接池优化的是数据访问侧的并发能力。建议配合读写分离，将 /api/count 等只读探针路由到只读副本。连接池耗尽的典型症状是请求延迟陡增且无错误日志，需通过指标接口观察等待队列长度。

# 多实例部署
对于大规模部署，可启动多个服务实例并通过负载均衡分发流量，实例间通过共享缓存与共享数据库保持一致。与单机部署不同，多实例需特别注意会话状态与缓存一致性，推荐使用 Redis 作为共享缓存后端。配置上各实例共享同一 DATABASE_URL，但可分配不同的实例标识用于日志追踪。扩缩容通过负载均衡器动态调整，无需停机。注意避免多实例重复执行全量同步造成写放大。

# 监控与告警
服务暴露 /api/metrics 运行指标，配合告警规则在 QPS 异常或错误率超阈值时触发通知。告警阈值通过配置文件设定，例如连续 5 分钟错误率高于 1% 即告警。与日志配置仅影响输出不同，监控告警是主动发现问题的重要手段。建议将告警接入统一通知渠道。指标采集频率由采集端决定，服务本身只负责暴露数据。历史指标应落库以便回溯容量变化。
`

// goldenCase 一条黄金问答：query 对应知识库中某个小节的唯一标识 needle。
// 注意：部分 query 故意指向「被硬负例章节包围」的目标章节（如启动类、接口类），
// 这类问题纯向量检索容易把语义相近的硬负例排到答案前面，是 rerank 最该发挥作用的场景。
type goldenCase struct {
	query  string
	needle string
}

var goldenCases = []goldenCase{
	// —— 启动类（三个章节语义高度相近，考查 rerank 能否识别 start.sh 才是答案）——
	{"怎样用脚本启动这个服务？", "start.sh"},
	{"用 Docker 怎么启动这个服务？", "docker compose"},
	{"怎么把服务注册成系统服务启动？", "systemctl"},
	// —— 接口类（多个 /api/ 接口语义相近，考查能否区分 count/list/search/metrics）——
	{"怎么查询已安装应用的总数？", "/api/count"},
	{"返回运行指标和 QPS 的接口是哪个？", "/api/metrics"},
	{"分页获取应用列表用哪个接口？", "/api/list"},
	{"按关键字搜索应用用哪个接口？", "/api/search"},
	// —— 平台采集类（Windows 双路径 / macOS / Linux 相互干扰）——
	{"Windows 上怎么读取已安装应用？", "注册表"},
	{"Windows 用 WMI 怎么枚举已安装产品？", "Win32_Product"},
	{"macOS 上用什么命令获取应用信息？", "system_profiler"},
	{"Linux 下通过什么获取应用列表？", "dpkg"},
	// —— 配置类（PORT 与 DATABASE_URL / LOG_LEVEL 易混）——
	{"怎么修改服务监听的端口？", "PORT"},
	{"数据库连接串配哪个环境变量？", "DATABASE_URL"},
	{"怎么调整日志输出级别？", "LOG_LEVEL"},
	// —— 排查类（端口占用 vs 数据库连接超时）——
	{"启动失败提示端口被占用怎么排查？", "address already in use"},
	{"启动报数据库连接超时怎么排查？", "connection timeout"},
	// —— 机制类 ——
	{"服务是怎么把各平台数据同步进库的？", "增量更新"},
	{"所有请求怎么鉴权？", "鉴权 Token"},
	{"怎么加速关键字检索？", "倒排索引"},
	{"多实例部署怎样保持一致？", "共享缓存"},
}

func TestRAGRecallBeforeAfter(t *testing.T) {
	apiKey := os.Getenv("EMBEDDING_API_KEY")
	if apiKey == "" {
		t.Skip("EMBEDDING_API_KEY 未设置，跳过召回率评估（需要百炼 embedding + rerank）")
	}

	// 让相对路径的 config/config.toml 可被找到（go test 的 cwd 是 test/ 目录，需回退到仓库根）
	repoRoot := findRepoRoot()
	if repoRoot == "" {
		t.Skip("找不到 config/config.toml，跳过召回率评估")
	}
	_ = os.Chdir(repoRoot)

	weaviateHost := os.Getenv("WEAVIATE_HOST")
	if weaviateHost == "" {
		weaviateHost = "localhost:8082"
	}

	// 注入测试用配置
	c := config.GetConfig()
	c.WeaviateConfig.WeaviateHost = weaviateHost
	c.RagModelConfig.RagEmbeddingModel = getEnv("RAG_EMBEDDING_MODEL", "text-embedding-v4")
	c.RagModelConfig.RagBaseUrl = getEnv("RAG_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	c.RagModelConfig.RagDimension = 1024
	c.RagModelConfig.RagHybridAlpha = 0.5
	c.RagModelConfig.RagRetrieveK = 20
	c.RagModelConfig.RagRerankTopK = 5
	c.RagModelConfig.RagRerankModel = "gte-rerank-v2" // 启用 eino 重排

	ctx := context.Background()

	// 准备独立的测试 class，避免污染真实数据
	filename := fmt.Sprintf("recalltest_%d.md", time.Now().UnixNano())
	className := rag.GenerateClassName(filename)

	idx, err := rag.NewRAGIndexer(filename, c.RagModelConfig.RagEmbeddingModel)
	if err != nil {
		t.Fatalf("创建 indexer 失败: %v", err)
	}
	defer func() { _ = rag.DeleteIndex(context.Background(), filename) }()

	tmpPath := writeTempFile(t, goldenDoc)
	defer os.Remove(tmpPath)
	if err := idx.IndexFile(ctx, tmpPath); err != nil {
		t.Fatalf("索引文档失败: %v", err)
	}

	// 后档检索器：带 reranker（配置 RagRerankModel 非空）
	qAfter, err := rag.NewRAGQueryWithClass(ctx, className)
	if err != nil {
		t.Fatalf("创建增强检索器失败: %v", err)
	}
	// 中档检索器：临时关掉 rerank，构造「混合无重排」对照（隔离 rerank 自身贡献）
	c.RagModelConfig.RagRerankModel = ""
	qMid, err := rag.NewRAGQueryWithClass(ctx, className)
	if err != nil {
		t.Fatalf("创建混合检索器失败: %v", err)
	}
	c.RagModelConfig.RagRerankModel = "gte-rerank-v2" // 恢复，避免影响其他逻辑

	// eval 对给定检索函数跑完整个黄金集，返回 recall@1/@3/@5 与 MRR，以及每条 query 的 needle 排名。
	eval := func(retrieve func(query string) ([]*schema.Document, error)) (r1, r3, r5, mrr float64, ranks map[string]int) {
		ranks = make(map[string]int, len(goldenCases))
		hits1, hits3, hits5 := 0, 0, 0
		rrSum := 0.0
		for _, cs := range goldenCases {
			docs, e := retrieve(cs.query)
			if e != nil {
				t.Logf("检索失败 query=%q: %v", cs.query, e)
				ranks[cs.query] = -1
				continue
			}
			rank := rankOfNeedle(docs, cs.needle)
			ranks[cs.query] = rank
			if rank >= 0 {
				if rank < 1 {
					hits1++
				}
				if rank < 3 {
					hits3++
				}
				if rank < 5 {
					hits5++
				}
				rrSum += 1.0 / float64(rank+1)
			}
		}
		n := float64(len(goldenCases))
		return float64(hits1) / n, float64(hits3) / n, float64(hits5) / n, rrSum / n, ranks
	}

	before1, before3, before5, beforeMRR, _ := eval(func(query string) ([]*schema.Document, error) {
		return qAfter.RetrieveDocuments(ctx, query) // 朴素路径不读 reranker 字段，复用任一 q 均可
	})
	mid1, mid3, mid5, midMRR, ranksMid := eval(func(query string) ([]*schema.Document, error) {
		return qMid.RetrieveDocumentsEnhanced(ctx, query)
	})
	after1, after3, after5, afterMRR, ranksAfter := eval(func(query string) ([]*schema.Document, error) {
		return qAfter.RetrieveDocumentsEnhanced(ctx, query)
	})

	t.Logf("================ RAG 召回率对比（黄金集 %d 条，混合 alpha=%.2f, retrieveK=%d, rerankTopK=%d）================",
		len(goldenCases), c.RagModelConfig.RagHybridAlpha, c.RagModelConfig.RagRetrieveK, c.RagModelConfig.RagRerankTopK)
	t.Logf("%-14s | %-10s | %-10s | %-10s | %-10s", "方法", "recall@1", "recall@3", "recall@5", "MRR")
	t.Logf("%-14s | %-10.3f | %-10.3f | %-10.3f | %-10.3f", "朴素(前)", before1, before3, before5, beforeMRR)
	t.Logf("%-14s | %-10.3f | %-10.3f | %-10.3f | %-10.3f", "混合(中)", mid1, mid3, mid5, midMRR)
	t.Logf("%-14s | %-10.3f | %-10.3f | %-10.3f | %-10.3f", "混合+重排(后)", after1, after3, after5, afterMRR)
	t.Logf("================================================================================")

	// 逐条排名变化：隔离 rerank 贡献（混合→混合+重排）
	improved, dropped, same := 0, 0, 0
	for _, cs := range goldenCases {
		rb, ra := ranksMid[cs.query], ranksAfter[cs.query]
		if rb < 0 || ra < 0 {
			continue
		}
		// rank 越小越靠前；-1 表示未命中，此处已跳过
		if ra < rb {
			improved++
		} else if ra > rb {
			dropped++
		} else {
			same++
		}
	}
	t.Logf("rerank 贡献（混合→混合+重排）：排名提升 %d 条、下降 %d 条、不变 %d 条", improved, dropped, same)

	// 朴素→增强的整体收益
	if after1 > before1 {
		t.Logf("结论：混合+重排在 recall@1 上更优（关键文档更靠前）")
	}
	if after5 >= before5 {
		t.Logf("结论：混合+重排在 recall@5 上不弱于朴素检索（无召回退化）")
	}
	if mid5 > 0 && after5 >= mid5 && after1 >= mid1 {
		t.Logf("结论：在混合检索基础上，重排未引入退化（recall@1/@5 均不弱于纯混合）")
	}

	// 健全性断言：若朴素与增强的 recall@5 都为 0，说明检索链路本身坏了。
	if before5 == 0 && after5 == 0 {
		t.Fatalf("朴素与增强检索的 recall@5 均为 0，检索链路异常，请检查 Weaviate / embedding 配置")
	}
}

// rankOfNeedle 返回第一个包含 needle 的文档下标；找不到返回 -1。
func rankOfNeedle(docs []*schema.Document, needle string) int {
	for i, d := range docs {
		if strings.Contains(d.Content, needle) {
			return i
		}
	}
	return -1
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "rag_recall_*.md")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	tmp.Close()
	return tmp.Name()
}

// findRepoRoot 从当前目录向上查找包含 config/config.toml 的仓库根目录。
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 6; i++ {
		if _, e := os.Stat(filepath.Join(dir, "config", "config.toml")); e == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
