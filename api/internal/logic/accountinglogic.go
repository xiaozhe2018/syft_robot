package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/qx/syft_robot/api/internal/model"
	"github.com/qx/syft_robot/api/internal/svc"
)

type AccountingLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAccountingLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AccountingLogic {
	return &AccountingLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// StartAccounting 开始一个新的记账周期
func (l *AccountingLogic) StartAccounting(chatID int64, userID int64, income float64) error {
	// 检查是否已有活跃的记账周期
	activeKey := fmt.Sprintf("accounting:active:%d:%d", chatID, userID)
	activeID, err := l.svcCtx.Redis.Get(activeKey)
	if err == nil && activeID != "" {
		// 结束现有周期
		if err := l.EndAccounting(chatID, userID); err != nil {
			return fmt.Errorf("结束现有记账周期失败: %v", err)
		}
	}

	// 创建新的记账周期
	now := time.Now()
	endTime := now.AddDate(0, 0, 7) // 默认一周后结束

	cycle := &model.AccountingCycle{
		ID:        uuid.New().String(),
		ChatID:    chatID,
		UserID:    userID,
		StartTime: now,
		EndTime:   endTime,
		Income:    income,
		Records:   make([]*model.AccountingRecord, 0),
		IsActive:  true,
		CreatedAt: now,
	}

	// 保存到Redis
	cycleKey := fmt.Sprintf("accounting:cycle:%s", cycle.ID)
	data, err := json.Marshal(cycle)
	if err != nil {
		return fmt.Errorf("序列化记账周期失败: %v", err)
	}

	if err := l.svcCtx.Redis.Set(cycleKey, string(data)); err != nil {
		return fmt.Errorf("保存记账周期失败: %v", err)
	}

	// 设置为活跃周期
	if err := l.svcCtx.Redis.Set(activeKey, cycle.ID); err != nil {
		return fmt.Errorf("设置活跃周期失败: %v", err)
	}

	// 添加到历史记录
	l.addToHistory(chatID, userID, cycle.ID)

	// 发送确认消息
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
		"✅ 已开始新的记账周期！\n"+
			"📅 起始日期: %s\n"+
			"💰 收入金额: %.2f 元\n"+
			"⏰ 结束日期: %s\n\n"+
			"使用回复(Reply)方式输入每日支出金额即可记账",
		now.Format("2006-01-02"),
		income,
		endTime.Format("2006-01-02"),
	))
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

// EndAccounting 结束当前记账周期
func (l *AccountingLogic) EndAccounting(chatID int64, userID int64) error {
	// 获取当前活跃的记账周期
	activeKey := fmt.Sprintf("accounting:active:%d:%d", chatID, userID)
	cycleID, err := l.svcCtx.Redis.Get(activeKey)
	if err != nil || cycleID == "" {
		return fmt.Errorf("找不到活跃的记账周期")
	}

	// 获取周期详情
	cycle, err := l.getAccountingCycle(cycleID)
	if err != nil {
		return err
	}

	// 设置为非活跃
	cycle.IsActive = false
	cycle.EndTime = time.Now()

	// 保存更新后的周期
	if err := l.saveAccountingCycle(cycle); err != nil {
		return err
	}

	// 添加到历史记录
	if err := l.addToHistory(chatID, userID, cycleID); err != nil {
		// 仅记录错误，不中断流程
		fmt.Printf("添加到历史记录失败: %v\n", err)
	}

	// 清除活跃标记
	_, err = l.svcCtx.Redis.Del(activeKey)
	if err != nil {
		return fmt.Errorf("清除活跃周期标记失败: %v", err)
	}

	// 发送统计信息
	summary := l.calculateSummary(cycle)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf(
		"📊 记账周期已结束！\n"+
			"📅 周期: %s 至 %s\n"+
			"💰 总收入: %.2f 元\n"+
			"💸 总支出: %.2f 元\n"+
			"💵 剩余金额: %.2f 元\n\n"+
			"使用 /accounting_history 查看所有历史记录",
		cycle.StartTime.Format("2006-01-02"),
		cycle.EndTime.Format("2006-01-02"),
		summary.TotalIncome,
		summary.TotalExpense,
		summary.Balance,
	))
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

// AddExpense 添加支出记录
func (l *AccountingLogic) AddExpense(chatID int64, userID int64, amount float64, description string) error {
	// 获取当前活跃的记账周期
	activeKey := fmt.Sprintf("accounting:active:%d:%d", chatID, userID)
	cycleID, err := l.svcCtx.Redis.Get(activeKey)
	if err != nil || cycleID == "" {
		return fmt.Errorf("找不到活跃的记账周期，请先使用 /accounting_start 命令开始记账")
	}

	// 获取周期详情
	cycle, err := l.getAccountingCycle(cycleID)
	if err != nil {
		return err
	}

	// 添加支出记录
	now := time.Now()
	record := &model.AccountingRecord{
		Amount:      amount,  // 直接使用传入的金额，不再取负
		Description: description,
		Date:        now,
		CreatedAt:   now,
	}
	cycle.Records = append(cycle.Records, record)

	// 保存更新后的周期
	if err := l.saveAccountingCycle(cycle); err != nil {
		return err
	}

	// 计算摘要
	summary := l.calculateSummary(cycle)

	// 发送确认消息
	var msgText string
	if amount < 0 {
		msgText = fmt.Sprintf("✅ 已记录支出: %.2f 元 - %s\n\n", -amount, description)
	} else {
		msgText = fmt.Sprintf("✅ 已记录收入: %.2f 元 - %s\n\n", amount, description)
	}
	
	msgText += fmt.Sprintf(
		"📊 当前统计:\n"+
		"💰 总收入: %.2f 元\n"+
		"💸 总支出: %.2f 元\n"+
		"💵 剩余金额: %.2f 元\n"+
		"⏰ 剩余天数: %d 天",
		summary.TotalIncome,
		summary.TotalExpense,
		summary.Balance,
		summary.DaysRemaining)
	
	msg := tgbotapi.NewMessage(chatID, msgText)
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

// GetAccountingSummary 获取当前记账周期的摘要
func (l *AccountingLogic) GetAccountingSummary(chatID int64, userID int64) error {
	// 获取当前活跃的记账周期
	activeKey := fmt.Sprintf("accounting:active:%d:%d", chatID, userID)
	cycleID, err := l.svcCtx.Redis.Get(activeKey)
	if err != nil || cycleID == "" {
		return fmt.Errorf("找不到活跃的记账周期，请先使用 /accounting_start 命令开始记账")
	}

	// 获取周期详情
	cycle, err := l.getAccountingCycle(cycleID)
	if err != nil {
		return err
	}

	// 计算摘要
	summary := l.calculateSummary(cycle)

	// 构建消息文本
	var msgText strings.Builder
	msgText.WriteString(fmt.Sprintf(
		"📊 当前记账周期统计:\n"+
			"📅 开始日期: %s\n"+
			"⏰ 结束日期: %s\n"+
			"💰 总收入: %.2f 元\n"+
			"💸 总支出: %.2f 元\n"+
			"💵 剩余金额: %.2f 元\n"+
			"⏰ 剩余天数: %d 天\n\n",
		cycle.StartTime.Format("2006-01-02"),
		cycle.EndTime.Format("2006-01-02"),
		summary.TotalIncome,
		summary.TotalExpense,
		summary.Balance,
		summary.DaysRemaining,
	))

	// 添加记录明细
	if len(cycle.Records) > 0 {
		msgText.WriteString("📝 记账明细:\n")
		for i, record := range cycle.Records {
			if record.Amount < 0 {
				// 支出记录
				msgText.WriteString(fmt.Sprintf("%d. %s - 支出 %.2f 元 - %s\n", 
					i+1, 
					record.Date.Format("01-02 15:04"), 
					-record.Amount,
					record.Description,
				))
			} else {
				// 收入记录
				msgText.WriteString(fmt.Sprintf("%d. %s - 收入 %.2f 元 - %s\n", 
					i+1, 
					record.Date.Format("01-02 15:04"), 
					record.Amount,
					record.Description,
				))
			}
		}
	} else {
		msgText.WriteString("暂无记账记录")
	}

	// 发送摘要消息
	msg := tgbotapi.NewMessage(chatID, msgText.String())
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

// 获取记账周期
func (l *AccountingLogic) getAccountingCycle(cycleID string) (*model.AccountingCycle, error) {
	cycleKey := fmt.Sprintf("accounting:cycle:%s", cycleID)
	data, err := l.svcCtx.Redis.Get(cycleKey)
	if err != nil {
		return nil, fmt.Errorf("获取记账周期失败: %v", err)
	}
	if data == "" {
		return nil, fmt.Errorf("记账周期不存在")
	}

	var cycle model.AccountingCycle
	if err := json.Unmarshal([]byte(data), &cycle); err != nil {
		return nil, fmt.Errorf("解析记账周期失败: %v", err)
	}

	return &cycle, nil
}

// 保存记账周期
func (l *AccountingLogic) saveAccountingCycle(cycle *model.AccountingCycle) error {
	cycleKey := fmt.Sprintf("accounting:cycle:%s", cycle.ID)
	data, err := json.Marshal(cycle)
	if err != nil {
		return fmt.Errorf("序列化记账周期失败: %v", err)
	}

	return l.svcCtx.Redis.Set(cycleKey, string(data))
}

// 计算摘要
func (l *AccountingLogic) calculateSummary(cycle *model.AccountingCycle) *model.AccountingSummary {
	summary := &model.AccountingSummary{
		TotalIncome: cycle.Income,  // 初始收入
		TotalExpense: 0,
		Balance: cycle.Income,
	}

	// 计算总收入和总支出
	for _, record := range cycle.Records {
		if record.Amount > 0 {
			summary.TotalIncome += record.Amount
		} else {
			summary.TotalExpense += -record.Amount  // 支出是负数，取负后为正
		}
	}
	
	// 计算余额
	summary.Balance = summary.TotalIncome - summary.TotalExpense

	// 计算剩余天数
	now := time.Now()
	if cycle.EndTime.After(now) {
		summary.DaysRemaining = int(cycle.EndTime.Sub(now).Hours() / 24)
	} else {
		summary.DaysRemaining = 0
	}

	return summary
}

// HasActiveAccountingCycle 检查用户是否有活跃的记账周期
func (l *AccountingLogic) HasActiveAccountingCycle(chatID int64, userID int64) (bool, error) {
	// 获取当前活跃的记账周期ID
	activeKey := fmt.Sprintf("accounting:active:%d:%d", chatID, userID)
	cycleID, err := l.svcCtx.Redis.Get(activeKey)
	if err != nil {
		return false, err
	}
	
	// 如果没有活跃周期ID，返回false
	if cycleID == "" {
		return false, nil
	}
	
	// 尝试获取周期详情，确认是否有效
	_, err = l.getAccountingCycle(cycleID)
	if err != nil {
		// 如果获取失败，说明记录可能已经被删除
		// 清除活跃标记
		_, _ = l.svcCtx.Redis.Del(activeKey)
		return false, nil
	}
	
	return true, nil
}

// GetAccountingHistory 获取用户的历史记账记录
func (l *AccountingLogic) GetAccountingHistory(chatID int64, userID int64) error {
	// 获取用户所有的记账周期
	historyKey := fmt.Sprintf("accounting:history:%d:%d", chatID, userID)
	data, err := l.svcCtx.Redis.Get(historyKey)
	if err != nil {
		return fmt.Errorf("获取历史记录失败: %v", err)
	}
	
	var cycleIDs []string
	if data != "" {
		if err := json.Unmarshal([]byte(data), &cycleIDs); err != nil {
			return fmt.Errorf("解析历史记录失败: %v", err)
		}
	}
	
	if len(cycleIDs) == 0 {
		msg := tgbotapi.NewMessage(chatID, "您还没有任何记账记录")
		_, err = l.svcCtx.Bot.Send(msg)
		return err
	}
	
	// 构建消息
	var msgText strings.Builder
	msgText.WriteString("📜 历史记账记录:\n\n")
	
	// 创建内联键盘按钮
	var keyboard [][]tgbotapi.InlineKeyboardButton
	
	// 按周期顺序显示简要信息
	for i, cycleID := range cycleIDs {
		cycle, err := l.getAccountingCycle(cycleID)
		if err != nil {
			// 如果某个周期获取失败，跳过
			continue
		}
		
		// 计算周期摘要
		summary := l.calculateSummary(cycle)
		
		msgText.WriteString(fmt.Sprintf("%d. 周期 %s 至 %s\n", 
			i+1,
			cycle.StartTime.Format("2006-01-02"),
			cycle.EndTime.Format("2006-01-02")))
		msgText.WriteString(fmt.Sprintf("   收入: %.2f元, 支出: %.2f元, 余额: %.2f元\n\n", 
			summary.TotalIncome,
			summary.TotalExpense,
			summary.Balance))
		
		// 为每个周期创建一个按钮
		button := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("查看周期 %s 详情", cycle.StartTime.Format("01-02")),
			fmt.Sprintf("view_cycle_%s", cycleID),
		)
		keyboard = append(keyboard, []tgbotapi.InlineKeyboardButton{button})
	}
	
	msg := tgbotapi.NewMessage(chatID, msgText.String())
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

// GetAccountingCycleById 根据ID获取并显示特定记账周期的详情
func (l *AccountingLogic) GetAccountingCycleById(chatID int64, cycleID string) error {
	// 获取周期详情
	cycle, err := l.getAccountingCycle(cycleID)
	if err != nil {
		return fmt.Errorf("获取记账周期失败: %v", err)
	}
	
	// 计算摘要
	summary := l.calculateSummary(cycle)
	
	// 构建消息文本
	var msgText strings.Builder
	msgText.WriteString(fmt.Sprintf(
		"📊 记账周期详情:\n"+
			"📅 周期: %s 至 %s\n"+
			"💰 总收入: %.2f 元\n"+
			"💸 总支出: %.2f 元\n"+
			"💵 最终余额: %.2f 元\n\n",
		cycle.StartTime.Format("2006-01-02"),
		cycle.EndTime.Format("2006-01-02"),
		summary.TotalIncome,
		summary.TotalExpense,
		summary.Balance,
	))
	
	// 添加记录明细
	if len(cycle.Records) > 0 {
		msgText.WriteString("📝 记账明细:\n")
		for i, record := range cycle.Records {
			if record.Amount < 0 {
				// 支出记录
				msgText.WriteString(fmt.Sprintf("%d. %s - 支出 %.2f 元 - %s\n", 
					i+1, 
					record.Date.Format("01-02 15:04"), 
					-record.Amount,
					record.Description,
				))
			} else {
				// 收入记录
				msgText.WriteString(fmt.Sprintf("%d. %s - 收入 %.2f 元 - %s\n", 
					i+1, 
					record.Date.Format("01-02 15:04"), 
					record.Amount,
					record.Description,
				))
			}
		}
	} else {
		msgText.WriteString("暂无记账记录")
	}
	
	// 发送消息
	msg := tgbotapi.NewMessage(chatID, msgText.String())
	_, err = l.svcCtx.Bot.Send(msg)
	return err
}

// 添加记账周期到历史记录
func (l *AccountingLogic) addToHistory(chatID int64, userID int64, cycleID string) error {
	historyKey := fmt.Sprintf("accounting:history:%d:%d", chatID, userID)
	
	// 获取现有历史记录
	data, err := l.svcCtx.Redis.Get(historyKey)
	var cycleIDs []string
	
	if err == nil && data != "" {
		if err := json.Unmarshal([]byte(data), &cycleIDs); err != nil {
			return fmt.Errorf("解析历史记录失败: %v", err)
		}
	}
	
	// 检查是否已存在
	for _, id := range cycleIDs {
		if id == cycleID {
			return nil // 已存在，不需要添加
		}
	}
	
	// 添加新的周期ID
	cycleIDs = append(cycleIDs, cycleID)
	
	// 保存到Redis
	updatedData, err := json.Marshal(cycleIDs)
	if err != nil {
		return fmt.Errorf("序列化历史记录失败: %v", err)
	}
	
	return l.svcCtx.Redis.Set(historyKey, string(updatedData))
} 