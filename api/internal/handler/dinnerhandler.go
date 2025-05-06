package handler

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/logic"
	"github.com/qx/syft_robot/api/internal/svc"
)

type DinnerHandler struct {
	svcCtx *svc.ServiceContext
	dinnerLogic *logic.DinnerLogic
}

func NewDinnerHandler(svcCtx *svc.ServiceContext, dinnerLogic *logic.DinnerLogic) *DinnerHandler {
	return &DinnerHandler{
		svcCtx: svcCtx,
		dinnerLogic: dinnerLogic,
	}
}

func (h *DinnerHandler) HandleUpdate(update tgbotapi.Update) error {
	if update.CallbackQuery != nil {
		return h.handleCallback(update.CallbackQuery)
	}

	if update.Message != nil {
		return h.handleMessage(update.Message)
	}

	return nil
}

func (h *DinnerHandler) handleCallback(callback *tgbotapi.CallbackQuery) error {
	data := callback.Data
	chatID := callback.Message.Chat.ID
	userID := callback.From.ID

	// 检查回调数据是否以 dinner_signup_ 开头
	if strings.HasPrefix(data, "dinner_signup_") {
		// 提取按钮中的用户ID
		parts := strings.Split(data, "_")
		if len(parts) != 3 {
			return fmt.Errorf("invalid callback data format: %s", data)
		}
		buttonUserID, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid user ID in callback data: %s", parts[2])
		}

		// 获取当前报名信息
		key := fmt.Sprintf("dinner:%d", chatID)
		dinner, err := h.dinnerLogic.GetDinner(key)
		if err != nil {
			return err
		}

		// 检查用户是否已报名
		_, isSignedUp := dinner.UserSignups[userID]

		// 如果是取消按钮，需要验证权限
		if isSignedUp && buttonUserID != userID {
			msg := tgbotapi.NewMessage(chatID, "请不要操作其他人的报名")
			_, err = h.svcCtx.Bot.Send(msg)
			return err
		}

		return h.dinnerLogic.HandleDinnerSignup(chatID, userID, callback.From.FirstName)
	}

	return fmt.Errorf("unknown callback data: %s", data)
}

func (h *DinnerHandler) handleMessage(message *tgbotapi.Message) error {
	if !message.IsCommand() {
		return nil
	}

	command := message.Command()
	chatID := message.Chat.ID
	userID := message.From.ID

	switch command {
	case "start":
		msg := tgbotapi.NewMessage(chatID, "欢迎使用晚餐报名机器人！\n使用 /dinner 开始今天的报名")
		_, err := h.svcCtx.Bot.Send(msg)
		return err

	case "help":
		msg := tgbotapi.NewMessage(chatID, "可用命令：\n/dinner - 开始今天的晚餐报名\n/cancel - 取消当前报名（仅发起人可用）\n/quit - 取消自己的报名")
		_, err := h.svcCtx.Bot.Send(msg)
		return err

	case "dinner":
		h.dinnerLogic.AddGroupID(chatID)
		return h.dinnerLogic.StartDinner(chatID, userID)

	case "cancel":
		return h.dinnerLogic.CancelDinner(chatID, userID)

	case "quit":
		return h.dinnerLogic.QuitDinner(chatID, userID, message.From.FirstName)

	default:
		msg := tgbotapi.NewMessage(chatID, "未知命令，请使用 /help 查看可用命令")
		_, err := h.svcCtx.Bot.Send(msg)
		return err
	}
} 