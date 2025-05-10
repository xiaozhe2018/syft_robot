package types

import "time"

// AccountingRecord 记账记录
type AccountingRecord struct {
	UserID      int64     `json:"user_id"`
	Amount      float64   `json:"amount"`
	Description string    `json:"description"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
} 