package svc

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/config"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
	Config config.Config
	Bot    *tgbotapi.BotAPI
	Redis  *redis.Redis
}

func NewServiceContext(c config.Config) *ServiceContext {
	bot, err := tgbotapi.NewBotAPI(c.Bot.Token)
	if err != nil {
		panic(err)
	}

	redisClient := redis.MustNewRedis(c.Redis)
	
	return &ServiceContext{
		Config: c,
		Bot:    bot,
		Redis:  redisClient,
	}
} 