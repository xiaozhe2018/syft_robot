package model

type Dinner struct {
	ID          string         `json:"id"`
	ChatID      int64         `json:"chat_id"`
	CreatorID   int64         `json:"creator_id"`
	Menu        []string      `json:"menu"`
	SignCount   int           `json:"sign_count"`
	Signups     []*DinnerSignup `json:"signups"`
	UserSignups map[int64]int64 `json:"user_signups"`
	CreatedAt   int64         `json:"created_at"`
	UpdatedAt   int64         `json:"updated_at"`
}

type DinnerSignup struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	Time      int64  `json:"time"`
}

var DefaultMenu = []string{
	"🍚 炒青菜",
	"🍜 炖肉",
	"🥗 炒牛肉",
	"其他家常菜..",
} 