package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
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
		log.Printf("Redis中没有找到群组ID数据")
		return
	}

	groupMu.Lock()
	defer groupMu.Unlock()
	if err := json.Unmarshal([]byte(data), &groupIDs); err != nil {
		log.Printf("解析群组ID失败: %v", err)
		return
	}
	
	log.Printf("成功从Redis加载群组ID: %v", groupIDs)
}

type DinnerLogic struct {
	svcCtx         *svc.ServiceContext
	accountingLogic *AccountingLogic
}

func NewDinnerLogic(svcCtx *svc.ServiceContext) *DinnerLogic {
	return &DinnerLogic{
		svcCtx:         svcCtx,
		accountingLogic: NewAccountingLogic(context.Background(), svcCtx),
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

// 清理无效的群组ID
func (l *DinnerLogic) cleanInvalidGroupIDs() {
	groupMu.Lock()
	defer groupMu.Unlock()
	
	invalidGroups := make([]int64, 0)
	
	// 检查每个群组
	for chatID := range groupIDs {
		log.Printf("正在检查群组 %d", chatID)
		// 尝试发送一条测试消息
		msg := tgbotapi.NewMessage(chatID, "测试消息")
		_, err := l.svcCtx.Bot.Send(msg)
		if err != nil {
			log.Printf("群组 %d 无效，将被移除: %v", chatID, err)
			invalidGroups = append(invalidGroups, chatID)
		} else {
			log.Printf("群组 %d 有效", chatID)
		}
	}
	
	// 移除无效的群组
	for _, chatID := range invalidGroups {
		delete(groupIDs, chatID)
		log.Printf("已移除无效群组 %d", chatID)
	}
	
	// 保存更新后的群组列表到Redis
	key := "bot:groups"
	data, err := json.Marshal(groupIDs)
	if err != nil {
		log.Printf("保存群组ID失败: %v", err)
		return
	}
	err = l.svcCtx.Redis.Set(key, string(data))
	if err != nil {
		log.Printf("保存群组ID到Redis失败: %v", err)
		return
	}
	
	log.Printf("当前有效的群组列表: %v", groupIDs)
	log.Printf("已清理 %d 个无效群组", len(invalidGroups))
}

func (l *DinnerLogic) StartReminder(testMode bool) {
	// 加载已保存的群组ID
	l.LoadGroupIDs()
	
	// 清理无效的群组ID
	l.cleanInvalidGroupIDs()
	
	// 打印当前群组数量
	groupMu.RLock()
	groupCount := len(groupIDs)
	groupMu.RUnlock()
	log.Printf("已加载 %d 个群组", groupCount)
	
	ticker := time.NewTicker(time.Minute)
	if testMode {
		// 测试模式下使用10秒间隔
		ticker = time.NewTicker(10 * time.Second)
		log.Printf("测试模式已启动，每10秒发送一次提醒")
	}
	
	go func() {
		for range ticker.C {
			now := time.Now()
			hour := now.Hour()
			minute := now.Minute()

			// 测试模式下不检查时间，直接发送提醒
			if testMode {
				// 向所有群组发送提醒
				groupMu.RLock()
				groupCount := len(groupIDs)
				log.Printf("测试模式：准备向 %d 个群组发送提醒", groupCount)
				
				invalidGroups := make([]int64, 0)
				for chatID := range groupIDs {
					log.Printf("测试模式：正在向群组 %d 发送提醒", chatID)
					msg := tgbotapi.NewMessage(chatID,
						fmt.Sprintf("⏰ 测试模式提醒\n\n"+
							"深夜饭堂提醒大家：\n"+
							"该喝水了 💧\n"+
							"该摸鱼了 🐟\n"+
							"该抽烟了 🚬\n\n"+
							"工作是人家的，命是自己的！\n"+
							"每天8杯水，bug减一半——亲测无效，但至少能续命！"))
					_, err := l.svcCtx.Bot.Send(msg)
					if err != nil {
						log.Printf("测试模式：向群组 %d 发送提醒失败: %v", chatID, err)
						invalidGroups = append(invalidGroups, chatID)
					} else {
						log.Printf("测试模式：成功向群组 %d 发送提醒", chatID)
					}
				}
				groupMu.RUnlock()
				
				// 如果有无效群组，移除它们
				if len(invalidGroups) > 0 {
					groupMu.Lock()
					for _, chatID := range invalidGroups {
						delete(groupIDs, chatID)
						log.Printf("已移除无效群组 %d", chatID)
					}
					groupMu.Unlock()
					
					// 保存更新后的群组列表到Redis
					key := "bot:groups"
					data, err := json.Marshal(groupIDs)
					if err == nil {
						l.svcCtx.Redis.Set(key, string(data))
						log.Printf("已更新Redis中的群组列表: %v", groupIDs)
					}
				}
				
				continue
			}

			// 正常模式下检查是否在指定时间段内（9-12点和14-18点）
			if ((hour >= 9 && hour < 12) || (hour >= 14 && hour < 18)) && minute == 0 {
				// 向所有群组发送提醒
				groupMu.RLock()
				groupCount := len(groupIDs)
				log.Printf("正常模式：准备向 %d 个群组发送提醒", groupCount)
				
				invalidGroups := make([]int64, 0)
				for chatID := range groupIDs {
					log.Printf("正常模式：正在向群组 %d 发送提醒", chatID)
					msg := tgbotapi.NewMessage(chatID,
						fmt.Sprintf("⏰ 当前时间为%d时%d分\n\n"+
							"深夜饭堂提醒大家：\n"+
							"该喝水了 💧\n"+
							"该摸鱼了 🐟\n"+
							"该抽烟了 🚬\n\n"+
							"工作是人家的，命是自己的！\n"+
							"每天8杯水，bug减一半——亲测无效，但至少能续命！", hour, minute))
					_, err := l.svcCtx.Bot.Send(msg)
					if err != nil {
						log.Printf("正常模式：向群组 %d 发送提醒失败: %v", chatID, err)
						invalidGroups = append(invalidGroups, chatID)
					} else {
						log.Printf("正常模式：成功向群组 %d 发送提醒", chatID)
					}
				}
				groupMu.RUnlock()
				
				// 如果有无效群组，移除它们
				if len(invalidGroups) > 0 {
					groupMu.Lock()
					for _, chatID := range invalidGroups {
						delete(groupIDs, chatID)
						log.Printf("已移除无效群组 %d", chatID)
					}
					groupMu.Unlock()
					
					// 保存更新后的群组列表到Redis
					key := "bot:groups"
					data, err := json.Marshal(groupIDs)
					if err == nil {
						l.svcCtx.Redis.Set(key, string(data))
						log.Printf("已更新Redis中的群组列表: %v", groupIDs)
					}
				}
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

func parseExpenseAmountAndDescription(text string) (float64, string, error) {
	// 移除所有空格
	text = strings.ReplaceAll(text, " ", "")
	
	// 尝试匹配带括号的格式
	re := regexp.MustCompile(`^(.+?)([+-]?\d+(?:\.\d+)?)(?:\((.+?)\))?$`)
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 3 {
		description := matches[1]
		amountStr := matches[2]
		note := ""
		if len(matches) > 3 && matches[3] != "" {
			note = "(" + matches[3] + ")"
		}
		
		// 如果金额字符串包含负号，直接按负数处理
		if strings.Contains(amountStr, "-") {
			amountStr = strings.Replace(amountStr, "-", "", 1)
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				return -amount, description + note, nil
			}
		} else if strings.Contains(amountStr, "+") {
			// 如果包含加号，按正数处理
			amountStr = strings.Replace(amountStr, "+", "", 1)
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				return amount, description + note, nil
			}
		} else {
			// 如果没有符号，默认为支出（负数）
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				return -amount, description + note, nil
			}
		}
	}
	
	// 如果没有匹配到带括号的格式，尝试其他格式
	parts := strings.Split(text, "-")
	if len(parts) == 2 {
		description := parts[0]
		amount, err := strconv.ParseFloat(parts[1], 64)
		if err == nil {
			return -amount, description, nil
		}
	}
	
	// 尝试匹配标准格式：金额 描述
	parts = strings.Split(text, " ")
	if len(parts) == 2 {
		amount, err := strconv.ParseFloat(parts[0], 64)
		if err == nil {
			// 如果没有负号，默认为支出（负数）
			return -amount, parts[1], nil
		}
	}
	
	return 0, "", fmt.Errorf("无法解析金额和描述，请使用以下格式之一：\n1. 100 午餐（默认为支出）\n2. 午餐-100\n3. -100 午餐\n4. 午餐 -100\n5. 买菜-10(未报销)\n6. 工资+5000（收入需要加+号）")
}

// HandleExpenseReply 处理支出回复
func (l *DinnerLogic) HandleExpenseReply(chatID int64, userID int64, text string) error {
	amount, description, err := parseExpenseAmountAndDescription(text)
	if err != nil {
		return err
	}

	// 添加记录
	if err := l.accountingLogic.AddExpense(chatID, userID, amount, description); err != nil {
		return err
	}

	return nil
} 