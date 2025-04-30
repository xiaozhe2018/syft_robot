package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/model"
	"github.com/qx/syft_robot/api/internal/svc"
)

type DinnerLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDinnerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DinnerLogic {
	return &DinnerLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DinnerLogic) StartDinner(chatID int64, adminID int64) error {
	// 检查是否已有进行中的报名
	key := fmt.Sprintf("dinner:%d", chatID)
	exists, err := l.svcCtx.Redis.Exists(key)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("当前已有进行中的报名")
	}

	// 创建新的报名
	dinner := &model.Dinner{
		GroupID:     chatID,
		Menu:        model.DefaultMenu,
		SignCount:   0,
		Signups:     make(map[int64]model.SignupInfo),
		StartTime:   time.Now(),
		AdminID:     adminID,
		UserSignups: make(map[int64]time.Time),
	}

	// 保存到 Redis
	if err := l.saveDinner(key, dinner); err != nil {
		return err
	}

	// 发送初始消息
	msg := tgbotapi.NewMessage(chatID, "<b>🍽️ 开始今天的晚餐报名！</b>")
	msg.ParseMode = "HTML"
	_, err = l.svcCtx.Bot.Send(msg)
	if err != nil {
		return err
	}

	// 发送菜单
	return l.sendMenu(chatID)
}

func (l *DinnerLogic) CancelDinner(chatID int64, userID int64) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		return err
	}

	if dinner.AdminID != userID {
		return fmt.Errorf("只有报名发起人才能取消报名")
	}

	// 删除报名信息
	_, err = l.svcCtx.Redis.Del(key)
	return err
}

func (l *DinnerLogic) Signup(chatID int64, userID int64, userName string) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		return err
	}

	// 检查用户是否已经报名过
	if lastSignup, exists := dinner.UserSignups[userID]; exists {
		if isSameDay(lastSignup, time.Now()) {
			return fmt.Errorf("您今天已经报名过了，请明天再来")
		}
	}

	// 更新报名信息
	dinner.Signups[userID] = model.SignupInfo{
		UserName: userName,
		Time:     time.Now(),
	}
	dinner.UserSignups[userID] = time.Now()
	dinner.SignCount++

	// 保存更新
	if err := l.saveDinner(key, dinner); err != nil {
		return err
	}

	// 更新菜单显示
	return l.sendMenu(chatID)
}

func (l *DinnerLogic) sendMenu(chatID int64) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		return err
	}

	// 创建菜单消息
	menuText := "<b>📋 今日菜单：</b>\n\n"
	for _, dish := range dinner.Menu {
		menuText += fmt.Sprintf("<code>%s</code>\n", dish)
	}
	menuText += fmt.Sprintf("\n当前报名人数：<code>%d</code>人", dinner.SignCount)

	// 创建按钮
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				"✅ 我要报名",
				"signup",
			),
		},
	}

	// 发送菜单消息
	msg := tgbotapi.NewMessage(chatID, menuText)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

func (l *DinnerLogic) GetDinner(key string) (*model.Dinner, error) {
	data, err := l.svcCtx.Redis.Get(key)
	if err != nil {
		return nil, err
	}
	if data == "" {
		return nil, fmt.Errorf("未找到报名信息")
	}

	var dinner model.Dinner
	if err := json.Unmarshal([]byte(data), &dinner); err != nil {
		return nil, err
	}
	return &dinner, nil
}

func (l *DinnerLogic) saveDinner(key string, dinner *model.Dinner) error {
	data, err := json.Marshal(dinner)
	if err != nil {
		return err
	}
	return l.svcCtx.Redis.Set(key, string(data))
}

func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
} 