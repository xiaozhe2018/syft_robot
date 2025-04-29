package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	mu            sync.Mutex
	active        = make(map[int64]*Dinner)    // 群组ID → 晚餐数据
	userSignups   = make(map[int64]time.Time)  // 用户ID → 最后报名时间
	defaultMenu   = []string{"🍚 炒青菜", "🍜 炖肉", "🥗 炒牛肉","其他家常菜.."}
)

type Dinner struct {
	GroupID     int64
	Menu        []string
	SignCount   int
	Signups     map[int64]SignupInfo  // 用户ID → 报名信息
	StartTime   time.Time
	AdminID     int64
	UserSignups map[int64]time.Time  // 用户ID → 报名时间
}

type SignupInfo struct {
	DishIndex int
	UserName  string
	Time      time.Time
}

func main() {
	bot, err := tgbotapi.NewBotAPI("your key")
	if err != nil {
		log.Fatal(err)
	}
	bot.Debug = true

	// 设置日志输出格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

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
	_, err = bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		log.Printf("设置命令失败: %v", err)
	}

	// 获取机器人信息
	me, err := bot.GetMe()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("机器人已启动: @%s", me.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		log.Printf("收到更新: %+v", update)
		
		if update.Message != nil {
			log.Printf("收到消息: 群组ID=%d, 用户ID=%d, 文本=%s", 
				update.Message.Chat.ID, 
				update.Message.From.ID,
				update.Message.Text)
			
			// 处理命令
			if update.Message.IsCommand() {
				log.Printf("检测到命令: %s", update.Message.Command())
				switch update.Message.Command() {
				case "start":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, 
						"欢迎使用深夜饭堂机器人！\n" +
						"使用以下命令：\n" +
						"/dinner - 开始今天的晚餐报名\n" +
						"/help - 显示帮助信息")
					_, err := bot.Send(msg)
					if err != nil {
						log.Printf("发送欢迎消息失败: %v", err)
					}
				case "help":
					msg := tgbotapi.NewMessage(update.Message.Chat.ID,
						"📖 深夜饭堂机器人使用帮助：\n\n" +
						"1. 管理员命令：\n" +
						"   /dinner - 发起新的晚餐报名\n" +
						"   /cancel - 取消当前报名（仅发起人可用）\n\n" +
						"2. 报名规则：\n" +
						"   - 每人每天只能报名一次\n" +
						"   - 报名后不可取消\n" +
						"   - 报名信息会在群内实时更新\n\n" +
						"3. 其他命令：\n" +
						"   /help - 显示本帮助信息")
					_, err := bot.Send(msg)
					if err != nil {
						log.Printf("发送帮助消息失败: %v", err)
					}
				case "cancel":
					mu.Lock()
					dinner := active[update.Message.Chat.ID]
					mu.Unlock()

					if dinner == nil {
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⚠️ 当前没有进行中的报名。")
						_, err := bot.Send(msg)
						if err != nil {
							log.Printf("发送消息失败: %v", err)
						}
						return
					}

					if dinner.AdminID != update.Message.From.ID {
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⚠️ 只有报名发起人才能取消报名。")
						_, err := bot.Send(msg)
						if err != nil {
							log.Printf("发送消息失败: %v", err)
						}
						return
					}

					// 删除报名信息
					mu.Lock()
					delete(active, update.Message.Chat.ID)
					mu.Unlock()

					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "✅ 报名已取消。")
					_, err := bot.Send(msg)
					if err != nil {
						log.Printf("发送消息失败: %v", err)
					}
				case "dinner":
					log.Printf("处理 dinner 命令")
					startDinner(bot, update.Message.Chat.ID, update.Message.From.ID)
				}
			}
		}

		// 处理按钮点击
		if update.CallbackQuery != nil {
			log.Printf("收到按钮点击: 用户ID=%d, 数据=%s", 
				update.CallbackQuery.From.ID,
				update.CallbackQuery.Data)
			handleCallback(bot, update.CallbackQuery)
		}
	}
}

// 启动晚餐报名
func startDinner(bot *tgbotapi.BotAPI, chatID int64, adminID int64) {
	mu.Lock()
	defer mu.Unlock()

	// 检查是否已有进行中的报名
	if _, exists := active[chatID]; exists {
		msg := tgbotapi.NewMessage(chatID, "⚠️ 当前已有进行中的报名，请等待当前报名结束后再发起新的报名。")
		_, err := bot.Send(msg)
		if err != nil {
			log.Printf("发送警告消息失败: %v", err)
		}
		return
	}

	// 创建新的报名
	now := time.Now()
	dinner := &Dinner{
		GroupID:     chatID,
		Menu:        defaultMenu,
		SignCount:   0,
		Signups:     make(map[int64]SignupInfo),
		StartTime:   now,
		AdminID:     adminID,
		UserSignups: make(map[int64]time.Time),
	}
	active[chatID] = dinner

	// 发送初始消息
	msg := tgbotapi.NewMessage(chatID, "<b>🍽️ 开始今天的晚餐报名！</b>")
	msg.ParseMode = "HTML"
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("发送初始消息失败: %v", err)
		return
	}

	// 立即发送菜单
	log.Printf("立即发送菜单到群组 %d", chatID)
	
	// 创建菜单消息
	menuText := "<b>📋 今日菜单：</b>\n\n"
	for _, dish := range dinner.Menu {
		menuText += fmt.Sprintf("<code>%s</code>\n", dish)
	}
	menuText += "\n请点击下方按钮报名："

	// 只创建一个报名按钮
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				"✅ 我要报名",
				"signup",
			),
		},
	}

	// 发送菜单消息
	menuMsg := tgbotapi.NewMessage(chatID, menuText)
	menuMsg.ParseMode = "HTML"
	menuMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	
	_, err = bot.Send(menuMsg)
	if err != nil {
		log.Printf("发送菜单消息失败: %v", err)
		return
	}
	
	log.Printf("菜单消息发送成功")
}

// 发送带按钮的菜单
func sendMenu(bot *tgbotapi.BotAPI, chatID int64) error {
	mu.Lock()
	dinner := active[chatID]
	mu.Unlock()

	if dinner == nil {
		log.Printf("未找到群组 %d 的报名信息", chatID)
		return fmt.Errorf("未找到报名信息")
	}

	// 统计每个菜品的报名人数和报名者
	counts := make(map[int]int)
	users := make(map[int][]string)
	for _, info := range dinner.Signups {
		counts[info.DishIndex]++
		users[info.DishIndex] = append(users[info.DishIndex], info.UserName)
	}

	// 创建菜单消息
	menuText := "📋 今日菜单：\n\n"
	for i, dish := range dinner.Menu {
		count := counts[i]
		menuText += fmt.Sprintf("%s (%d人)\n", dish, count)
		if count > 0 {
			menuText += "报名成员：\n"
			for _, userName := range users[i] {
				menuText += fmt.Sprintf("  👤 %s\n", userName)
			}
		}
		menuText += "\n"
	}

	// 创建按钮
	buttons := make([][]tgbotapi.InlineKeyboardButton, len(dinner.Menu))
	for i, dish := range dinner.Menu {
		buttons[i] = []tgbotapi.InlineKeyboardButton{
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("报名 %s", dish),
				fmt.Sprintf("signup_%d", i),
			),
		}
	}

	log.Printf("准备发送菜单消息到群组 %d", chatID)
	
	// 创建消息配置
	msg := tgbotapi.NewMessage(chatID, menuText)
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	
	// 发送消息
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("发送菜单消息失败: %v", err)
		return err
	}
	
	log.Printf("菜单消息发送成功")
	return nil
}

// 处理按钮点击
func handleCallback(bot *tgbotapi.BotAPI, callback *tgbotapi.CallbackQuery) {
	mu.Lock()
	defer mu.Unlock()

	log.Printf("收到报名请求: 用户=%s, 群组ID=%d", callback.From.FirstName, callback.Message.Chat.ID)

	dinner := active[callback.Message.Chat.ID]
	if dinner == nil {
		// 发送报名已取消的提示消息
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, 
			"<b>⚠️ 本次报名已取消，请等待新的报名发起！</b>")
		msg.ParseMode = "HTML"
		_, err := bot.Send(msg)
		if err != nil {
			log.Printf("发送报名已取消提示消息失败: %v", err)
		}
		return
	}

	// 检查用户是否已经报名过
	if lastSignup, exists := dinner.UserSignups[callback.From.ID]; exists {
		// 检查是否是同一天
		if isSameDay(lastSignup, time.Now()) {
			// 发送提示消息
			msg := tgbotapi.NewMessage(callback.Message.Chat.ID, 
				fmt.Sprintf("<b>⚠️ %s，您今天已经报名过了，请明天再来！</b>", callback.From.FirstName))
			msg.ParseMode = "HTML"
			_, err := bot.Send(msg)
			if err != nil {
				log.Printf("发送提示消息失败: %v", err)
			}
			return
		}
	}

	// 获取用户信息
	userName := callback.From.FirstName
	if callback.From.LastName != "" {
		userName += " " + callback.From.LastName
	}

	// 更新报名信息
	dinner.Signups[callback.From.ID] = SignupInfo{
		UserName:  userName,
		Time:      time.Now(),
	}
	dinner.UserSignups[callback.From.ID] = time.Now()
	dinner.SignCount++

	log.Printf("用户 %s 报名成功，当前报名人数：%d", userName, dinner.SignCount)

	// 发送报名成功消息到群组
	successMsg := tgbotapi.NewMessage(callback.Message.Chat.ID, 
		fmt.Sprintf("<b>✅ %s 报名成功！</b>\n当前报名人数：<code>%d</code>人", userName, dinner.SignCount))
	successMsg.ParseMode = "HTML"
	_, err := bot.Send(successMsg)
	if err != nil {
		log.Printf("发送报名成功消息失败: %v", err)
	}

	// 更新菜单显示
	menuText := "<b>📋 今日菜单：</b>\n\n"
	for _, dish := range dinner.Menu {
		menuText += fmt.Sprintf("<code>%s</code>\n", dish)
	}
	menuText += fmt.Sprintf("\n当前报名人数：<code>%d</code>人", dinner.SignCount)

	// 更新消息
	editMsg := tgbotapi.NewEditMessageText(
		callback.Message.Chat.ID,
		callback.Message.MessageID,
		menuText,
	)
	editMsg.ParseMode = "HTML"
	editMsg.ReplyMarkup = callback.Message.ReplyMarkup
	_, err = bot.Send(editMsg)
	if err != nil {
		log.Printf("更新菜单消息失败: %v", err)
	}

	// 确认回调查询
	callbackConfig := tgbotapi.NewCallback(callback.ID, "")
	_, err = bot.Request(callbackConfig)
	if err != nil {
		log.Printf("确认回调查询失败: %v", err)
	}
}

// 辅助函数：解析按钮索引
func parseIndex(data string) int {
	if len(data) < 7 {
		return -1
	}
	var index int
	fmt.Sscanf(data[7:], "%d", &index)
	return index
}

// 辅助函数：检查是否是同一天
func isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
