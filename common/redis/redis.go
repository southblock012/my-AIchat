package redis

import (
	"context"
	"my-AIchat/config"
	"strconv"
	"strings"
	"time"

	redisCli "github.com/redis/go-redis/v9"
)

var Rdb *redisCli.Client

var ctx = context.Background()

func InitRedis() error {
	conf := config.GetConfig()
	host := conf.RedisConfig.RedisHost
	port := conf.RedisConfig.RedisPort
	password := conf.RedisConfig.RedisPassword
	db := conf.RedisConfig.RedisDb
	addr := host + ":" + strconv.Itoa(port)

	Rdb = redisCli.NewClient(&redisCli.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		Protocol: 2,
	})

	if err := Rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	return nil
}

func SetCaptchaForEmail(email, captcha string) error {
	key := GenerateCaptcha(email)
	expire := 2 * time.Minute
	return Rdb.Set(ctx, key, captcha, expire).Err()
}

func CheckCaptchaForEmail(email, userInput string) (bool, error) {
	key := GenerateCaptcha(email)

	storedCaptcha, err := Rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redisCli.Nil {

			return false, nil
		}

		return false, err
	}

	if strings.EqualFold(storedCaptcha, userInput) {
		_ = Rdb.Del(ctx, key).Err()
		return true, nil
	}

	return false, nil
}
