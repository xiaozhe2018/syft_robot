package model

import "time"

type Dinner struct {
	GroupID     int64
	Menu        []string
	SignCount   int
	Signups     map[int64]SignupInfo
	StartTime   time.Time
	AdminID     int64
	UserSignups map[int64]time.Time
}

type SignupInfo struct {
	DishIndex int
	UserName  string
	Time      time.Time
}

var DefaultMenu = []string{
	"🍚 炒青菜",
	"🍜 炖肉",
	"🥗 炒牛肉",
	"其他家常菜..",
} 