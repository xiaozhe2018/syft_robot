package main

import (
	"flag"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/config"
	"github.com/qx/syft_robot/api/internal/handler"
	"github.com/qx/syft_robot/api/internal/logic"
	"github.com/qx/syft_robot/api/internal/svc"
	"github.com/zeromicro/go-zero/core/conf"
)

var (
	configFile = flag.String("f", "etc/dinner.yaml", "the config file")
	testMode   = flag.Bool("test", false, "test mode for reminder")
)

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	svcCtx := svc.NewServiceContext(c)
	
	// 创建 DinnerLogic 实例
	dinnerLogic := logic.NewDinnerLogic(svcCtx)
	
	handler := handler.NewDinnerHandler(svcCtx, dinnerLogic)

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
		{
			Command:     "accounting_start",
			Description: "开始记账周期",
		},
		{
			Command:     "accounting_end",
			Description: "结束当前记账周期",
		},
		{
			Command:     "accounting_status",
			Description: "查看当前账单记录",
		},
		{
			Command:     "accounting_expense",
			Description: "添加支出记录",
		},
		{
			Command:     "accounting_history",
			Description: "查看历史记账记录",
		},
	}
	_, err := svcCtx.Bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		log.Printf("设置命令失败: %v", err)
	}

	// 获取机器人信息
	me, err := svcCtx.Bot.GetMe()
	if err != nil {
		log.Fatalf("获取机器人信息失败: %v", err)
	}
	log.Printf("机器人已启动: @%s", me.UserName)

	// 启动定时提醒
	dinnerLogic.StartReminder(*testMode)

	// 开始接收更新
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := svcCtx.Bot.GetUpdatesChan(u)

	for update := range updates {
		log.Printf("收到更新: %+v", update)
		handler.HandleUpdate(update)
	}
} 