package handler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/logic"
	"github.com/qx/syft_robot/api/internal/svc"
)

type DinnerHandler struct {
	svcCtx         *svc.ServiceContext
	dinnerLogic    *logic.DinnerLogic
	accountingLogic *logic.AccountingLogic
	// 记录正在等待输入的用户
	waitingForExpenseAmount map[int64]bool
	waitingForIncomeAmount  map[int64]bool
}

func NewDinnerHandler(svcCtx *svc.ServiceContext, dinnerLogic *logic.DinnerLogic) *DinnerHandler {
	accountingLogic := logic.NewAccountingLogic(context.Background(), svcCtx)
	return &DinnerHandler{
		svcCtx:                 svcCtx,
		dinnerLogic:            dinnerLogic,
		accountingLogic:        accountingLogic,
		waitingForExpenseAmount: make(map[int64]bool),
		waitingForIncomeAmount:  make(map[int64]bool),
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
	
	// 处理查看记账周期详情按钮
	if strings.HasPrefix(data, "view_cycle_") {
		// 提取记账周期ID
		cycleID := strings.TrimPrefix(data, "view_cycle_")
		if cycleID == "" {
			// 回复一个通知
			callback.Message.Text = "无效的记账周期ID"
			h.svcCtx.Bot.Request(tgbotapi.NewCallback(callback.ID, "无效的记账周期ID"))
			return nil
		}
		
		// 确认收到回调
		h.svcCtx.Bot.Request(tgbotapi.NewCallback(callback.ID, "正在加载..."))
		
		// 获取并显示记账周期详情
		return h.accountingLogic.GetAccountingCycleById(chatID, cycleID)
	}

	return fmt.Errorf("unknown callback data: %s", data)
}

func (h *DinnerHandler) handleMessage(message *tgbotapi.Message) error {
	userID := message.From.ID

	// 处理回复消息 - 记录支出金额
	if message.ReplyToMessage != nil && h.waitingForExpenseAmount[userID] {
		delete(h.waitingForExpenseAmount, userID)
		return h.dinnerLogic.HandleExpenseReply(message.Chat.ID, userID, message.Text)
	}

	// 处理回复消息 - 记录收入金额
	if message.ReplyToMessage != nil && h.waitingForIncomeAmount[userID] {
		return h.handleIncomeReply(message)
	}

	// 处理普通回复消息 - 尝试解析金额进行记账
	if message.ReplyToMessage != nil && message.ReplyToMessage.From.IsBot {
		return h.handleAccountingMessage(message)
	}

	// 处理命令
	if message.IsCommand() {
		return h.handleCommand(message)
	}

	return nil
}

func (h *DinnerHandler) handleCommand(message *tgbotapi.Message) error {
	command := message.Command()
	chatID := message.Chat.ID
	userID := message.From.ID

	// 处理查看特定记账周期的命令
	if strings.HasPrefix(command, "accounting_view_") {
		// 从命令中提取记账周期ID
		cycleID := strings.TrimPrefix(command, "accounting_view_")
		if cycleID == "" {
			msg := tgbotapi.NewMessage(chatID, "无效的记账周期ID")
			_, err := h.svcCtx.Bot.Send(msg)
			return err
		}
		
		// 获取并显示记账周期详情
		return h.accountingLogic.GetAccountingCycleById(chatID, cycleID)
	}

	switch command {
	case "start":
		msg := tgbotapi.NewMessage(chatID, "欢迎使用晚餐报名机器人！\n使用 /dinner 开始今天的报名")
		_, err := h.svcCtx.Bot.Send(msg)
		return err

	case "help":
		msg := tgbotapi.NewMessage(chatID, "可用命令：\n"+
			"/dinner - 开始今天的晚餐报名\n"+
			"/cancel - 取消当前报名（仅发起人可用）\n"+
			"/quit - 取消自己的报名\n\n"+
			"记账功能：\n"+
			"/accounting_start - 开始记账周期\n"+
			"/accounting_expense - 添加支出记录\n"+
			"/accounting_end - 结束当前记账周期\n"+
			"/accounting_status - 查看当前账单记录\n\n"+
			"💡 提示：直接回复(Reply)机器人消息即可记录支出")
		_, err := h.svcCtx.Bot.Send(msg)
		return err

	case "dinner":
		h.dinnerLogic.AddGroupID(chatID)
		return h.dinnerLogic.StartDinner(chatID, userID)

	case "cancel":
		return h.dinnerLogic.CancelDinner(chatID, userID)

	case "quit":
		return h.dinnerLogic.QuitDinner(chatID, userID, message.From.FirstName)

	case "accounting_start":
		// 设置用户为等待输入收入金额状态
		h.waitingForIncomeAmount[userID] = true
		msg := tgbotapi.NewMessage(chatID, "请回复(Reply)本消息，输入本周期的收入金额")
		_, err := h.svcCtx.Bot.Send(msg)
		if err != nil {
			return err
		}
		return nil

	case "accounting_end":
		return h.accountingLogic.EndAccounting(chatID, userID)

	case "accounting_status":
		// 检查是否有活跃的记账周期
		if active, err := h.accountingLogic.HasActiveAccountingCycle(chatID, userID); err != nil || !active {
			msg := tgbotapi.NewMessage(chatID, "当前没有活跃的记账周期，请先使用 /accounting_start 命令开始新的记账周期，或使用 /accounting_history 查看历史记录")
			_, err := h.svcCtx.Bot.Send(msg)
			return err
		}
		
		return h.accountingLogic.GetAccountingSummary(chatID, userID)

	case "accounting_history":
		// 查看历史记账记录
		return h.accountingLogic.GetAccountingHistory(chatID, userID)

	case "accounting_expense":
		// 检查是否有活跃的记账周期
		if active, err := h.accountingLogic.HasActiveAccountingCycle(chatID, userID); err != nil || !active {
			msg := tgbotapi.NewMessage(chatID, "当前没有活跃的记账周期，请先使用 /accounting_start 命令开始新的记账周期")
			_, err := h.svcCtx.Bot.Send(msg)
			return err
		}
		
		// 设置用户为等待输入支出金额状态
		h.waitingForExpenseAmount[userID] = true
		msg := tgbotapi.NewMessage(chatID, "请回复(Reply)本消息，增加记录\n\n支持以下格式：\n1. 100 午餐\n2. 买菜-5\n3. -100 打车\n4. 工资+5000\n5. 买菜-10(未报销)\n6. 打车-50（公司报销）")
		_, err := h.svcCtx.Bot.Send(msg)
		if err != nil {
			return err
		}
		return nil

	default:
		msg := tgbotapi.NewMessage(chatID, "未知命令，请使用 /help 查看可用命令")
		_, err := h.svcCtx.Bot.Send(msg)
		return err
	}
}

// 处理收入金额回复
func (h *DinnerHandler) handleIncomeReply(message *tgbotapi.Message) error {
	chatID := message.Chat.ID
	userID := message.From.ID
	
	// 清除等待状态
	delete(h.waitingForIncomeAmount, userID)
	
	// 解析收入金额
	income, err := strconv.ParseFloat(message.Text, 64)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "请输入有效的金额数字")
		_, err := h.svcCtx.Bot.Send(msg)
		return err
	}
	
	// 开始记账周期
	return h.accountingLogic.StartAccounting(chatID, userID, income)
}

// 处理支出金额回复
func (h *DinnerHandler) handleExpenseReply(message *tgbotapi.Message) error {
	chatID := message.Chat.ID
	userID := message.From.ID
	
	// 清除等待状态
	delete(h.waitingForExpenseAmount, userID)
	
	// 解析支出金额和描述
	text := message.Text
	amount, description, err := parseExpenseAmountAndDescription(text)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, err.Error())
		_, err := h.svcCtx.Bot.Send(msg)
		return err
	}
	
	// 添加记录
	return h.accountingLogic.AddExpense(chatID, userID, amount, description)
}

// 解析支出金额和描述
func parseExpenseAmountAndDescription(text string) (float64, string, error) {
	// 支持多种格式：
	// 1. "100 午餐" - 标准格式
	// 2. "午餐-100" - 描述在前，金额在后
	// 3. "-100 午餐" - 带负号的金额
	// 4. "午餐 -100" - 描述在前，带负号的金额
	// 5. "买菜-10(未报销)" - 带括号备注
	// 6. "买菜-10（未报销）" - 带中文括号备注
	
	// 首先尝试标准格式（金额在前）
	parts := strings.SplitN(strings.TrimSpace(text), " ", 2)
	amount, err := strconv.ParseFloat(parts[0], 64)
	if err == nil {
		description := "未知项目"
		if len(parts) > 1 {
			description = strings.TrimSpace(parts[1])
		}
		return amount, description, nil
	}
	
	// 尝试其他格式
	// 1. 检查是否包含负号
	if strings.Contains(text, "-") {
		// 分割负号前后的内容
		parts := strings.SplitN(text, "-", 2)
		if len(parts) == 2 {
			// 尝试解析金额（可能包含括号备注）
			amountStr := parts[1]
			// 移除括号中的备注
			amountStr = regexp.MustCompile(`[（(].*?[)）]`).ReplaceAllString(amountStr, "")
			amountStr = strings.TrimSpace(amountStr)
			
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				// 获取描述和备注
				description := strings.TrimSpace(parts[0])
				if description == "" {
					description = "未知项目"
				}
				
				// 提取括号中的备注
				noteMatch := regexp.MustCompile(`[（(](.*?)[)）]`).FindStringSubmatch(text)
				if len(noteMatch) > 1 {
					description = fmt.Sprintf("%s(%s)", description, noteMatch[1])
				}
				
				return -amount, description, nil
			}
		}
	}
	
	// 2. 检查是否包含加号
	if strings.Contains(text, "+") {
		// 分割加号前后的内容
		parts := strings.SplitN(text, "+", 2)
		if len(parts) == 2 {
			// 尝试解析金额（可能包含括号备注）
			amountStr := parts[1]
			// 移除括号中的备注
			amountStr = regexp.MustCompile(`[（(].*?[)）]`).ReplaceAllString(amountStr, "")
			amountStr = strings.TrimSpace(amountStr)
			
			amount, err := strconv.ParseFloat(amountStr, 64)
			if err == nil {
				// 获取描述和备注
				description := strings.TrimSpace(parts[0])
				if description == "" {
					description = "未知项目"
				}
				
				// 提取括号中的备注
				noteMatch := regexp.MustCompile(`[（(](.*?)[)）]`).FindStringSubmatch(text)
				if len(noteMatch) > 1 {
					description = fmt.Sprintf("%s(%s)", description, noteMatch[1])
				}
				
				return amount, description, nil
			}
		}
	}
	
	// 如果所有格式都不匹配，返回错误
	return 0, "", fmt.Errorf("无法解析金额和描述")
}

// 提取消息中的金额
func extractAmountsFromMessage(text string) []struct {
	Amount      float64
	Description string
} {
	var results []struct {
		Amount      float64
		Description string
	}

	// 匹配格式：数字前有+或-号，或者支出/收入关键词附近的数字
	// 例如：+100, -50, 支出20, 收入30
	// 支持小数点
	expensePattern := regexp.MustCompile(`[-]?\d+(\.\d+)?`)
	parts := strings.Split(text, ",")
	
	for _, part := range parts {
		// 查找所有的数字
		matches := expensePattern.FindAllString(part, -1)
		for _, match := range matches {
			amount, err := strconv.ParseFloat(match, 64)
			if err == nil {
				// 如果是数字，判断是支出还是收入
				var description string
				
				// 默认情况下，如果数字前有-号，或者文本中包含"支出"关键词，则视为支出
				if strings.Contains(match, "-") || strings.Contains(part, "支出") {
					if amount > 0 {
						amount = -amount // 确保支出为负数
					}
					description = strings.TrimSpace(strings.ReplaceAll(part, match, ""))
					description = strings.ReplaceAll(description, "支出", "")
				} else if strings.Contains(match, "+") || strings.Contains(part, "收入") {
					// 如果数字前有+号，或者文本中包含"收入"关键词，则视为收入
					if amount < 0 {
						amount = -amount // 确保收入为正数
					}
					description = strings.TrimSpace(strings.ReplaceAll(part, match, ""))
					description = strings.ReplaceAll(description, "收入", "")
				} else {
					// 没有明确标识，默认为支出
					if amount > 0 {
						amount = -amount
					}
					description = strings.TrimSpace(strings.ReplaceAll(part, match, ""))
				}
				
				// 清理description中的无用字符
				description = strings.Trim(description, ":：,，、. ")
				if description == "" {
					description = "未知项目"
				}
				
				results = append(results, struct {
					Amount      float64
					Description string
				}{Amount: amount, Description: description})
			}
		}
	}
	
	return results
}

// 处理普通记账消息
func (h *DinnerHandler) handleAccountingMessage(message *tgbotapi.Message) error {
	chatID := message.Chat.ID
	userID := message.From.ID
	
	// 提取消息中的金额
	amountItems := extractAmountsFromMessage(message.Text)
	if len(amountItems) == 0 {
		// 没有找到金额，忽略
		return nil
	}
	
	// 添加所有找到的记账项目
	for _, item := range amountItems {
		if err := h.accountingLogic.AddExpense(chatID, userID, item.Amount, item.Description); err != nil {
			// 如果是因为没有活跃的记账周期而失败，告知用户
			if strings.Contains(err.Error(), "找不到活跃的记账周期") {
				msg := tgbotapi.NewMessage(chatID, "请先使用 /accounting_start 命令开始记账周期")
				_, _ = h.svcCtx.Bot.Send(msg)
				return err
			}
			return err
		}
	}
	
	return nil
} 