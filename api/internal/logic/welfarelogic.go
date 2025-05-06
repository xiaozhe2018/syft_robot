package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/qx/syft_robot/api/internal/model"
	"github.com/qx/syft_robot/api/internal/svc"
)

type WelfareLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	welfares map[string]*model.Welfare
}

func NewWelfareLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WelfareLogic {
	return &WelfareLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		welfares: make(map[string]*model.Welfare),
	}
}

// LoadWelfares 从JSON文件加载福利信息
func (l *WelfareLogic) LoadWelfares() error {
	// 读取JSON文件
	data, err := os.ReadFile("api/uploads/welfare.json")
	if err != nil {
		return fmt.Errorf("读取JSON文件失败: %v", err)
	}

	// 解析JSON
	var response model.WelfareListResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("解析JSON失败: %v", err)
	}

	// 存储到内存中
	for _, item := range response.Items {
		// 创建新的 Welfare 对象，避免引用同一个对象
		welfare := item
		l.welfares[item.ID] = &welfare
	}

	return nil
}

// GetWelfareList 获取福利列表
func (l *WelfareLogic) GetWelfareList() *model.WelfareListResponse {
	response := &model.WelfareListResponse{
		Items: make([]model.Welfare, 0, len(l.welfares)),
	}
	
	for _, welfare := range l.welfares {
		if len(welfare.Photos) > 0 {
			// 创建新的 Welfare 对象，避免引用同一个对象
			item := *welfare
			response.Items = append(response.Items, item)
		}
	}
	
	return response
}

// GetWelfareDetail 获取福利详情
func (l *WelfareLogic) GetWelfareDetail(id string) (*model.WelfareDetailResponse, error) {
	welfare, ok := l.welfares[id]
	if !ok {
		return nil, fmt.Errorf("未找到福利信息")
	}
	
	return &model.WelfareDetailResponse{
		Welfare: *welfare,
	}, nil
}

// HandleWelfareList 处理福利列表展示
func (l *WelfareLogic) HandleWelfareList(chatID int64) error {
	list := l.GetWelfareList()
	
	// 创建消息
	msg := tgbotapi.NewMessage(chatID, "💗 MM列表 ❤️")
	
	// 创建内联键盘
	var keyboard [][]tgbotapi.InlineKeyboardButton
	for _, item := range list.Items {
		if len(item.Photos) > 0 {
			// 添加按钮
			row := []tgbotapi.InlineKeyboardButton{
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf("%s (%d岁)", item.Name, item.Age),
					fmt.Sprintf("welfare_detail:%s", item.ID),
				),
			}
			keyboard = append(keyboard, row)
		}
	}
	
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	_, err := l.svcCtx.Bot.Send(msg)
	return err
}

// HandleWelfarePreview 处理福利预览
func (l *WelfareLogic) HandleWelfarePreview(chatID int64, welfareID string) error {
	detail, err := l.GetWelfareDetail(welfareID)
	if err != nil {
		return err
	}
	
	if len(detail.Photos) > 0 {
		// 发送第一张图片作为预览
		photoMsg := tgbotapi.NewPhoto(chatID, tgbotapi.FilePath(detail.Photos[0]))
		photoMsg.Caption = fmt.Sprintf("🌟 %s 的预览 🌟\n\n"+
			"年龄: %d岁\n"+
			"身高: %dcm\n"+
			"体重: %dkg\n\n"+
			"点击下方按钮查看完整图集",
			detail.Name, detail.Age, detail.Height, detail.Weight)
		
		// 创建查看完整图集的按钮
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData(
					"📸 查看完整图集",
					fmt.Sprintf("welfare_detail:%s", welfareID),
				),
			},
		}
		photoMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
		_, err = l.svcCtx.Bot.Send(photoMsg)
		return err
	}
	
	return fmt.Errorf("未找到图片")
}

// HandleWelfareDetail 处理福利详情展示
func (l *WelfareLogic) HandleWelfareDetail(chatID int64, welfareID string) error {
	detail, err := l.GetWelfareDetail(welfareID)
	if err != nil {
		return err
	}
	
	// 创建媒体组
	var mediaGroup []interface{}
	
	// 添加所有图片
	for i, photo := range detail.Photos {
		photoMsg := tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath(photo))
		if i == 0 {
			// 第一张图片添加标题
			photoMsg.Caption = fmt.Sprintf("🌟 %s 的详细信息 🌟\n\n"+
				"年龄: %d岁\n"+
				"身高: %dcm\n"+
				"体重: %dkg\n\n"+
				"个人简介:\n%s\n\n"+
				"套餐信息:\n",
				detail.Name, detail.Age, detail.Height, detail.Weight, detail.Description)
			
			// 添加套餐信息
			for _, pkg := range detail.Packages {
				photoMsg.Caption += fmt.Sprintf("%s: %dP/%s%d次", pkg.Name, pkg.Price, pkg.Duration, pkg.Times)
				if pkg.Note != "" {
					photoMsg.Caption += fmt.Sprintf(" (%s)", pkg.Note)
				}
				photoMsg.Caption += "\n"
			}
		}
		mediaGroup = append(mediaGroup, photoMsg)
	}
	
	// 添加所有视频
	for _, video := range detail.Videos {
		videoMsg := tgbotapi.NewInputMediaVideo(tgbotapi.FilePath(video))
		mediaGroup = append(mediaGroup, videoMsg)
	}
	
	// 发送媒体组
	if len(mediaGroup) > 0 {
		mediaGroupMsg := tgbotapi.NewMediaGroup(chatID, mediaGroup)
		_, err := l.svcCtx.Bot.SendMediaGroup(mediaGroupMsg)
		if err != nil {
			return fmt.Errorf("发送媒体组失败: %v", err)
		}
		
		// 发送带按钮的消息
		msg := tgbotapi.NewMessage(chatID, "请选择操作：")
		keyboard := [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.NewInlineKeyboardButtonData(
					"🚗 立即上门",
					fmt.Sprintf("welfare_order:%s", welfareID),
				),
			},
		}
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
		_, err = l.svcCtx.Bot.Send(msg)
		if err != nil {
			return fmt.Errorf("发送按钮消息失败: %v", err)
		}
		return nil
	}
	
	return fmt.Errorf("未找到媒体文件")
}

// HandleWelfareOrder 处理福利订单
func (l *WelfareLogic) HandleWelfareOrder(callback *tgbotapi.CallbackQuery) error {
	// 解析福利ID
	welfareID := callback.Data[len("welfare_order:"):]
	log.Printf("处理上门请求: 福利ID=%s, 用户=%s", welfareID, callback.From.FirstName)

	// 获取福利信息
	welfare, exists := l.welfares[welfareID]
	if !exists {
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ 未找到该福利信息")
		l.svcCtx.Bot.Send(msg)
		return fmt.Errorf("未找到福利信息")
	}

	// 创建支付按钮
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonData(
				"💳 立即支付100U押金",
				fmt.Sprintf("welfare_pay:%s", welfareID),
			),
		},
	}

	// 发送押金支付提示
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, 
		fmt.Sprintf("💵 <b>押金支付</b>\n\n"+
			"为了确保服务质量，需要先支付100U押金。\n"+
			"支付完成后，我们会安排上门服务。\n\n"+
			"福利信息：\n"+
			"名称：%s\n"+
			"年龄：%s\n"+
			"身高：%s\n"+
			"体重：%s\n\n"+
			"请点击下方按钮支付押金：",
			welfare.Name, welfare.Age, welfare.Height, welfare.Weight))
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	l.svcCtx.Bot.Send(msg)

	// 发送回调响应
	callbackResponse := tgbotapi.NewCallback(callback.ID, "请支付押金")
	l.svcCtx.Bot.Send(callbackResponse)
	return nil
}

// HandleWelfarePay 处理押金支付
func (l *WelfareLogic) HandleWelfarePay(callback *tgbotapi.CallbackQuery) error {
	// 解析福利ID
	welfareID := callback.Data[len("welfare_pay:"):]
	log.Printf("处理押金支付: 福利ID=%s, 用户=%s", welfareID, callback.From.FirstName)

	// 获取福利信息
	welfare, exists := l.welfares[welfareID]
	if !exists {
		msg := tgbotapi.NewMessage(callback.Message.Chat.ID, "⚠️ 未找到该福利信息")
		l.svcCtx.Bot.Send(msg)
		return fmt.Errorf("未找到福利信息")
	}

	// 创建支付链接
	payURL := fmt.Sprintf("https://t.me/wallet?startattach=welfare_%s_%d", welfareID, callback.From.ID)

	// 创建支付按钮
	buttons := [][]tgbotapi.InlineKeyboardButton{
		{
			tgbotapi.NewInlineKeyboardButtonURL(
				"💳 打开Telegram钱包支付",
				payURL,
			),
		},
	}

	// 发送支付链接
	msg := tgbotapi.NewMessage(callback.Message.Chat.ID, 
		fmt.Sprintf("💵 <b>支付押金</b>\n\n"+
			"请点击下方按钮打开Telegram钱包支付100U押金。\n"+
			"支付完成后，我们会安排上门服务。\n\n"+
			"福利信息：\n"+
			"名称：%s\n"+
			"年龄：%s\n"+
			"身高：%s\n"+
			"体重：%s",
			welfare.Name, welfare.Age, welfare.Height, welfare.Weight))
	msg.ParseMode = "HTML"
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons...)
	l.svcCtx.Bot.Send(msg)

	// 发送回调响应
	callbackResponse := tgbotapi.NewCallback(callback.ID, "请打开Telegram钱包支付")
	l.svcCtx.Bot.Send(callbackResponse)
	return nil
}

// HandleWelfareOrderAddress 处理地址输入
func (l *WelfareLogic) HandleWelfareOrderAddress(chatID int64, welfareID string, address string) error {
	log.Printf("处理地址输入: 福利ID=%s, 地址=%s", welfareID, address)

	// 获取福利信息
	welfare, exists := l.welfares[welfareID]
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "⚠️ 未找到该福利信息")
		l.svcCtx.Bot.Send(msg)
		return fmt.Errorf("未找到福利信息")
	}

	// 发送确认消息
	msg := tgbotapi.NewMessage(chatID, 
		fmt.Sprintf("🚗 <b>已安排上门服务</b>\n\n"+
			"您的地址已收到，我们已安排车辆前往。\n\n"+
			"福利信息：\n"+
			"名称：%s\n"+
			"年龄：%s\n"+
			"身高：%s\n"+
			"体重：%s\n\n"+
			"预计到达时间：30分钟内\n"+
			"请保持电话畅通，司机将提前联系您。",
			welfare.Name, welfare.Age, welfare.Height, welfare.Weight))
	msg.ParseMode = "HTML"
	l.svcCtx.Bot.Send(msg)

	return nil
} 