package main

import (
	"flag"
	"log"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/qx/syft_robot/api/internal/config"
	"github.com/qx/syft_robot/api/internal/handler"
	"github.com/qx/syft_robot/api/internal/svc"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var configFile = flag.String("f", "etc/dinner.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	ctx := svc.NewServiceContext(c)
	handler := handler.NewDinnerHandler(ctx)

	// 设置命令列表
	commands := []tgbotapi.BotCommand{
		{
			Command:     "start",
			Description: "开始使用机器人",
		},
		{
			Command:     "help",
			Description: "显示帮助信息",
		},
		{
			Command:     "dinner",
			Description: "开始今天的晚餐报名",
		},
		{
			Command:     "cancel",
			Description: "取消当前报名（仅发起人可用）",
		},
	}
	_, err := ctx.Bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		log.Printf("设置命令失败: %v", err)
	}

	// 获取机器人信息
	me, err := ctx.Bot.GetMe()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("机器人已启动: @%s", me.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := ctx.Bot.GetUpdatesChan(u)

	for update := range updates {
		log.Printf("收到更新: %+v", update)
		handler.HandleUpdate(update)
	}
} 