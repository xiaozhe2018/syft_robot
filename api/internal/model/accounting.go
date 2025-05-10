package model

import (
	"time"
)

// AccountingCycle 表示一个记账周期
type AccountingCycle struct {
	ID        string                 `json:"id"`          // 唯一标识符
	ChatID    int64                  `json:"chat_id"`     // 聊天ID
	UserID    int64                  `json:"user_id"`     // 用户ID
	StartTime time.Time              `json:"start_time"`  // 开始时间
	EndTime   time.Time              `json:"end_time"`    // 结束时间（预计）
	Income    float64                `json:"income"`      // 周期内的收入
	Records   []*AccountingRecord    `json:"records"`     // 支出记录
	IsActive  bool                   `json:"is_active"`   // 是否是当前活跃的周期
	CreatedAt time.Time              `json:"created_at"`  // 创建时间
}

// AccountingRecord 表示一条支出记录
type AccountingRecord struct {
	Amount      float64    `json:"amount"`     // 金额（正数为收入，负数为支出）
	Description string     `json:"description"` // 描述
	Date        time.Time  `json:"date"`       // 日期
	CreatedAt   time.Time  `json:"created_at"` // 创建时间
}

// AccountingStartRequest 开始记账周期的请求
type AccountingStartRequest struct {
	Income float64 `json:"income"` // 本周期收入
}

// AccountingExpenseRequest 记录支出的请求
type AccountingExpenseRequest struct {
	Amount float64 `json:"amount"` // 支出金额
}

// AccountingSummary 记账周期的摘要
type AccountingSummary struct {
	TotalIncome   float64 `json:"total_income"`   // 总收入
	TotalExpense  float64 `json:"total_expense"`  // 总支出
	Balance       float64 `json:"balance"`        // 余额
	DaysRemaining int     `json:"days_remaining"` // 剩余天数
} 