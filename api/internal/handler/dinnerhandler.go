package handler

import (
	"context"
	"fmt"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/logic"
	"github.com/qx/syft_robot/api/internal/svc"
)

type DinnerHandler struct {
	svcCtx *svc.ServiceContext
}

func NewDinnerHandler(svcCtx *svc.ServiceContext) *DinnerHandler {
	return &DinnerHandler{
		svcCtx: svcCtx,
	}
}

func (h *DinnerHandler) HandleUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		h.handleMessage(update.Message)
	} else if update.CallbackQuery != nil {
		h.handleCallback(update.CallbackQuery)
	}
}

func (h *DinnerHandler) handleMessage(message *tgbotapi.Message) {
	if !message.IsCommand() {
		return
	}

	ctx := context.Background()
	logic := logic.NewDinnerLogic(ctx, h.svcCtx)

	switch message.Command() {
	case "start":
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"欢迎使用深夜饭堂机器人！\n"+
				"使用以下命令：\n"+
				"/dinner - 开始今天的晚餐报名\n"+
				"/help - 显示帮助信息")
		h.svcCtx.Bot.Send(msg)

	case "help":
		msg := tgbotapi.NewMessage(message.Chat.ID,
			"📖 深夜饭堂机器人使用帮助：\n\n"+
				"1. 管理员命令：\n"+
				"   /dinner - 发起新的晚餐报名\n"+
				"   /cancel - 取消当前报名（仅发起人可用）\n\n"+
				"2. 报名规则：\n"+
				"   - 每人每天只能报名一次\n"+
				"   - 报名后不可取消\n"+
				"   - 报名信息会在群内实时更新\n\n"+
				"3. 其他命令：\n"+
				"   /help - 显示本帮助信息")
		h.svcCtx.Bot.Send(msg)

	case "dinner":
		err := logic.StartDinner(message.Chat.ID, message.From.ID)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, "⚠️ "+err.Error())
			h.svcCtx.Bot.Send(msg)
		}

	case "cancel":
		err := logic.CancelDinner(message.Chat.ID, message.From.ID)
		if err != nil {
			msg := tgbotapi.NewMessage(message.Chat.ID, "⚠️ "+err.Error())
			h.svcCtx.Bot.Send(msg)
		} else {
			msg := tgbotapi.NewMessage(message.Chat.ID, "✅ 报名已取消。")
			h.svcCtx.Bot.Send(msg)
		}
	}
}

func (h *DinnerHandler) handleCallback(callback *tgbotapi.CallbackQuery) {
	ctx := context.Background()
	logic := logic.NewDinnerLogic(ctx, h.svcCtx)

	userName := callback.From.FirstName
	if callback.From.LastName != "" {
		userName += " " + callback.From.LastName
	}

	err := logic.Signup(callback.Message.Chat.ID, callback.From.ID, userName)
	if err != nil {
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ "+err.Error())
		h.svcCtx.Bot.Send(msg)
	} else {
		// 获取最新的报名人数
		key := fmt.Sprintf("dinner:%d", callback.Message.Chat.ID)
		dinner, err := logic.GetDinner(key)
		if err != nil {
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ "+err.Error())
			h.svcCtx.Bot.Send(msg)
			return
		}

		msg := tgbotapi.NewMessage(callback.Message.Chat.ID,
			fmt.Sprintf("<b>✅ %s 报名成功！</b>\n当前报名人数：<code>%d</code>人",
				userName, dinner.SignCount))
		msg.ParseMode = "HTML"
		h.svcCtx.Bot.Send(msg)
	}

	// 确认回调查询
	callbackConfig := tgbotapi.NewCallback(callback.ID, "")
	h.svcCtx.Bot.Request(callbackConfig)
} 