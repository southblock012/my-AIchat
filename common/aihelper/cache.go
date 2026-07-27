package aihelper

import (
	"encoding/json"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"my-AIchat/common/redis"
	"my-AIchat/dao/message"
	"my-AIchat/dao/usermemory"
	"my-AIchat/model"
)

const (
	ctxWindowSize   = 20               // 上下文窗口保留最近多少条消息（约 10 轮对话）
	ctxWindowTTL    = 30 * time.Minute // 窗口空闲存活时间，每次访问自动续期
	memProfileTTL   = 10 * time.Minute // 长期记忆 profile 缓存时间
	memProfileLimit = 20               // 注入 top-K 条记忆
)

func ctxWindowKey(sessionID string) string {
	return "session:" + sessionID + ":ctx"
}

func memProfileKey(userName string) string {
	return "user:" + userName + ":mem"
}

func toPtrSlice(msgs []model.Message) []*model.Message {
	out := make([]*model.Message, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		out = append(out, &m)
	}
	return out
}

// loadContextWindow 从 Redis 取最近上下文窗口；miss 时从 MySQL 回灌，Redis 不可用则降级返回空
func loadContextWindow(sessionID string) []*model.Message {
	data, err := redis.Rdb.Get(redis.Ctx, ctxWindowKey(sessionID)).Result()
	if err != nil {
		if err == goredis.Nil {
			// 缓存未命中：回灌 —— 从 MySQL 取全量历史最近 N 条写回 Redis
			if dbMsgs, derr := message.GetMessagesBySessionID(sessionID); derr == nil && len(dbMsgs) > 0 {
				if len(dbMsgs) > ctxWindowSize {
					dbMsgs = dbMsgs[len(dbMsgs)-ctxWindowSize:]
				}
				ptrs := toPtrSlice(dbMsgs)
				_ = saveContextWindow(sessionID, ptrs)
				return ptrs
			}
		}
		// Redis 不可用：降级，返回空（调用方只用当前问题，不崩溃）
		return nil
	}
	var msgs []model.Message
	if err := json.Unmarshal([]byte(data), &msgs); err != nil {
		return nil
	}
	return toPtrSlice(msgs)
}

// saveContextWindow 截断到最近 N 条并写回 Redis（TTL 续期）；Redis 不可用时静默失败
func saveContextWindow(sessionID string, msgs []*model.Message) error {
	if len(msgs) > ctxWindowSize {
		msgs = msgs[len(msgs)-ctxWindowSize:]
	}
	val := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		if m != nil {
			val = append(val, *m)
		}
	}
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return redis.Rdb.Set(redis.Ctx, ctxWindowKey(sessionID), string(data), ctxWindowTTL).Err()
}

// GetMemoryProfile 带缓存地获取长期记忆注入文本；Redis miss/错误时降级直接查库
func GetMemoryProfile(userName string) string {
	if data, err := redis.Rdb.Get(redis.Ctx, memProfileKey(userName)).Result(); err == nil {
		return data
	}
	prompt := usermemory.GetMemoryPrompt(userName, memProfileLimit)
	if prompt != "" {
		_ = redis.Rdb.Set(redis.Ctx, memProfileKey(userName), prompt, memProfileTTL).Err()
	}
	return prompt
}
