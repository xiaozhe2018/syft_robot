package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/qx/syft_robot/api/internal/model"
	"github.com/qx/syft_robot/api/internal/svc"
)

// 全局变量，用于存储群组ID
var (
	groupIDs = make(map[int64]bool)
	groupMu  sync.RWMutex
)

// AddGroupID 添加群组ID到全局变量和Redis
func (l *DinnerLogic) AddGroupID(chatID int64) {
	groupMu.Lock()
	defer groupMu.Unlock()
	groupIDs[chatID] = true

	// 将群组ID保存到Redis
	key := "bot:groups"
	data, err := json.Marshal(groupIDs)
	if err != nil {
		log.Printf("保存群组ID失败: %v", err)
		return
	}
	err = l.svcCtx.Redis.Set(key, string(data))
	if err != nil {
		log.Printf("保存群组ID到Redis失败: %v", err)
	}
}

// LoadGroupIDs 从Redis加载群组ID
func (l *DinnerLogic) LoadGroupIDs() {
	key := "bot:groups"
	data, err := l.svcCtx.Redis.Get(key)
	if err != nil {
		log.Printf("加载群组ID失败: %v", err)
		return
	}
	if data == "" {
		return
	}

	groupMu.Lock()
	defer groupMu.Unlock()
	if err := json.Unmarshal([]byte(data), &groupIDs); err != nil {
		log.Printf("解析群组ID失败: %v", err)
	}
}

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

func (l *DinnerLogic) StartDinner(chatID int64, userID int64) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	now := time.Now().Unix()

	// 检查是否已有进行中的报名
	exists, err := l.svcCtx.Redis.Exists(key)
	if err != nil {
		return err
	}
	if exists {
		msg := tgbotapi.NewMessage(chatID, "当前已有进行中的报名，请先取消后再重新发起")
		_, err = l.svcCtx.Bot.Send(msg)
		return err
	}

	// 创建新的晚餐信息
	dinner := &model.Dinner{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		CreatorID:   userID,
		Menu:        model.DefaultMenu,
		SignCount:   0,
		Signups:     make([]*model.DinnerSignup, 0),
		UserSignups: make(map[int64]int64),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 保存到Redis
	if err := l.saveDinner(key, dinner); err != nil {
		return err
	}

	// 发送初始消息
	msg := tgbotapi.NewMessage(chatID, "🍽️ 开始今天的晚餐报名！")
	msg.ParseMode = "HTML"
	_, err = l.svcCtx.Bot.Send(msg)
	if err != nil {
		return err
	}

	// 发送菜单
	return l.sendMenu(chatID, userID)
}

// updateMenu 根据报名人数更新菜单
func (l *DinnerLogic) updateMenu(dinner *model.Dinner) {
	// 基础菜单
	baseMenu := []string{
		"🍚 炒青菜",
		"🍜 炖肉",
		"🥗 炒牛肉",
	}

	// 根据报名人数添加菜品
	menu := make([]string, 0)
	menu = append(menu, baseMenu...)

	// 每增加2人，添加一个菜品
	additionalDishes := []string{
		"🥘 番茄炒蛋",
		"🍲 红烧鱼",
		"🥬 清炒时蔬",
		"🍗 宫保鸡丁",
		"🥩 回锅肉",
		"🍤 干锅虾",
		"🥘 麻婆豆腐",
		"🍲 水煮肉片",
	}

	// 计算需要添加的菜品数量
	additionalCount := (dinner.SignCount - 3) / 2
	if additionalCount > 0 {
		if additionalCount > len(additionalDishes) {
			additionalCount = len(additionalDishes)
		}
		menu = append(menu, additionalDishes[:additionalCount]...)
	}

	// 如果报名人数超过4人，添加一个汤
	if dinner.SignCount >= 4 {
		menu = append(menu, "🥣 紫菜蛋花汤")
	}

	dinner.Menu = menu
}

func (l *DinnerLogic) CancelDinner(chatID int64, userID int64) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		// 如果没有找到报名信息，发送提示消息
		msg := tgbotapi.NewMessage(chatID, "当前没有进行中的报名")
		_, err = l.svcCtx.Bot.Send(msg)
		return err
	}

	// 检查是否是发起人
	if dinner.CreatorID != userID {
		msg := tgbotapi.NewMessage(chatID, "只有报名发起人才能取消报名")
		_, err = l.svcCtx.Bot.Send(msg)
		return err
	}

	// 删除报名信息
	_, err = l.svcCtx.Redis.Del(key)
	if err != nil {
		return fmt.Errorf("取消报名失败: %v", err)
	}

	// 发送取消成功消息
	msg := tgbotapi.NewMessage(chatID, "✅ 报名已取消")
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

func (l *DinnerLogic) Signup(chatID int64, userID int64, firstName string) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.getDinnerInfo(key)
	if err != nil {
		return err
	}

	// 检查是否已经报名
	if _, exists := dinner.UserSignups[userID]; exists {
		return fmt.Errorf("您已经报名过了")
	}

	// 添加报名信息
	dinner.Signups = append(dinner.Signups, &model.DinnerSignup{
		UserID:    userID,
		FirstName: firstName,
		Time:      time.Now().Unix(),
	})
	dinner.UserSignups[userID] = time.Now().Unix()
	dinner.SignCount = len(dinner.Signups) // 更新报名人数
	dinner.UpdatedAt = time.Now().Unix()

	// 保存到Redis
	if err := l.saveDinner(key, dinner); err != nil {
		return err
	}

	// 发送成功消息
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("%s 报名成功！当前报名人数：%d人", firstName, dinner.SignCount))
	_, err = l.svcCtx.Bot.Send(msg)
	if err != nil {
		return err
	}

	// 更新菜单显示
	return l.sendMenu(chatID, userID)
}

func (l *DinnerLogic) sendMenu(chatID int64, userID int64) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		return err
	}

	// 更新菜单
	l.updateMenu(dinner)

	// 保存更新后的菜单
	if err := l.saveDinner(key, dinner); err != nil {
		return err
	}

	// 创建菜单消息
	var menuText strings.Builder
	menuText.WriteString("<b>📋 今日菜单：</b>\n\n")
	for _, dish := range dinner.Menu {
		menuText.WriteString(fmt.Sprintf("<code>%s</code>\n", dish))
	}
	menuText.WriteString(fmt.Sprintf("\n<b>👥 报名人员（%d人）：</b>\n", dinner.SignCount))
	
	// 添加报名人员列表
	if len(dinner.Signups) > 0 {
		for i, signup := range dinner.Signups {
			menuText.WriteString(fmt.Sprintf("%d. %s\n", i+1, signup.FirstName))
		}
	} else {
		menuText.WriteString("暂无报名人员\n")
	}

	// 根据用户是否已报名创建不同的按钮
	var buttonText string
	var callbackData string
	if _, exists := dinner.UserSignups[userID]; exists {
		buttonText = "❌ 我要取消"
		callbackData = fmt.Sprintf("dinner_signup_%d", userID) // 取消按钮需要用户ID
	} else {
		buttonText = "✅ 我要报名"
		callbackData = "dinner_signup_0" // 报名按钮使用公共ID
	}

	// 创建按钮
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				buttonText,
				callbackData,
			),
		},
	}

	// 发送菜单消息
	msg := tgbotapi.NewMessage(chatID, menuText.String())
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

func (l *DinnerLogic) getDinnerInfo(key string) (*model.Dinner, error) {
	data, err := l.svcCtx.Redis.Get(key)
	if err != nil {
		return nil, err
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

func (l *DinnerLogic) HandleDinnerSignup(chatID int64, userID int64, firstName string) error {
	// 获取报名信息
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		return fmt.Errorf("获取报名信息失败: %v", err)
	}

	// 检查是否已经报名
	if _, exists := dinner.UserSignups[userID]; exists {
		// 如果已经报名，则取消报名
		return l.QuitDinner(chatID, userID, firstName)
	}

	// 添加报名信息
	dinner.Signups = append(dinner.Signups, &model.DinnerSignup{
		UserID:    userID,
		FirstName: firstName,
		Time:      time.Now().Unix(),
	})
	dinner.UserSignups[userID] = time.Now().Unix()
	dinner.SignCount = len(dinner.Signups) // 更新报名人数
	dinner.UpdatedAt = time.Now().Unix()

	// 保存报名信息
	err = l.saveDinner(key, dinner)
	if err != nil {
		return fmt.Errorf("保存报名信息失败: %v", err)
	}

	// 更新菜单显示
	return l.sendMenu(chatID, userID)
}

func (l *DinnerLogic) StartReminder(testMode bool) {
	// 加载已保存的群组ID
	l.LoadGroupIDs()

	// 如果是测试模式，立即发送一次消息
	if testMode {
		now := time.Now()
		hour := now.Hour()
		minute := now.Minute()

		groupMu.RLock()
		for chatID := range groupIDs {
			msg := tgbotapi.NewMessage(chatID,
				fmt.Sprintf("⏰ 当前时间为%d时%d分\n\n"+
					"小哲提醒大家：\n"+
					"该喝水了 💧\n"+
					"该摸鱼了 🐟\n"+
					"该抽烟了 🚬\n\n"+
					"工作是人家的，命是自己的！\n"+
					"每天8杯水，bug减一半——亲测无效，但至少能续命！", hour, minute))
			l.svcCtx.Bot.Send(msg)
		}
		groupMu.RUnlock()
		return
	}

	// 创建一个定时器，每分钟检查一次
	ticker := time.NewTicker(time.Minute)
	go func() {
		for range ticker.C {
			now := time.Now()
			hour := now.Hour()
			minute := now.Minute()

			// 检查是否在指定时间段内（9-12点和14-18点）
			if ((hour >= 9 && hour < 12) || (hour >= 14 && hour < 18)) && minute == 0 {
				// 向所有群组发送提醒
				groupMu.RLock()
				for chatID := range groupIDs {
					msg := tgbotapi.NewMessage(chatID,
						fmt.Sprintf("⏰ 当前时间为%d时%d分\n\n"+
							"小哲提醒大家：\n"+
							"该喝水了 💧\n"+
							"该摸鱼了 🐟\n"+
							"该抽烟了 🚬\n\n"+
							"工作是人家的，命是自己的！\n"+
							"每天8杯水，bug减一半——亲测无效，但至少能续命！", hour, minute))
					l.svcCtx.Bot.Send(msg)
				}
				groupMu.RUnlock()
			}
		}
	}()
}

func (l *DinnerLogic) QuitDinner(chatID int64, userID int64, firstName string) error {
	key := fmt.Sprintf("dinner:%d", chatID)
	dinner, err := l.GetDinner(key)
	if err != nil {
		// 如果没有找到报名信息，发送提示消息
		msg := tgbotapi.NewMessage(chatID, "当前没有进行中的报名")
		_, err = l.svcCtx.Bot.Send(msg)
		return err
	}

	// 检查用户是否已报名
	if _, exists := dinner.UserSignups[userID]; !exists {
		msg := tgbotapi.NewMessage(chatID, "您还没有报名")
		_, err = l.svcCtx.Bot.Send(msg)
		return err
	}

	// 从报名列表中移除用户
	newSignups := make([]*model.DinnerSignup, 0)
	for _, signup := range dinner.Signups {
		if signup.UserID != userID {
			newSignups = append(newSignups, signup)
		}
	}
	dinner.Signups = newSignups

	// 从用户报名记录中移除
	delete(dinner.UserSignups, userID)

	// 更新报名人数
	dinner.SignCount = len(dinner.Signups)
	dinner.UpdatedAt = time.Now().Unix()

	// 保存更新后的报名信息
	if err := l.saveDinner(key, dinner); err != nil {
		return fmt.Errorf("保存报名信息失败: %v", err)
	}

	// 更新菜单显示
	return l.sendMenu(chatID, userID)
} 