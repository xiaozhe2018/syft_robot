# 深夜饭堂机器人

一个基于 Telegram Bot API 的晚餐报名机器人，用于管理群组内的晚餐报名活动。

## 功能特性

- 🍽️ 发起晚餐报名
- 📋 显示今日菜单
- 👥 实时统计报名人数
- ⏰ 每人每天只能报名一次
- 🚫 支持取消报名（仅发起人可用）
- 💬 使用 HTML 格式消息，防止消息被模仿

## 安装步骤

1. 克隆项目
```bash
git clone https://github.com/yourusername/syft.git
cd syft
```

2. 安装依赖
```bash
go mod download
```

3. 配置环境变量
```bash
export BOT_TOKEN="your_telegram_bot_token"
```

4. 运行程序
```bash
go run main.go
```

## 使用说明

### 命令列表

- `/start` - 开始使用机器人
- `/help` - 显示帮助信息
- `/dinner` - 发起新的晚餐报名
- `/cancel` - 取消当前报名（仅发起人可用）

### 报名流程

1. 管理员使用 `/dinner` 命令发起报名
2. 机器人会发送今日菜单和报名按钮
3. 群组成员点击"我要报名"按钮进行报名
4. 报名成功后，机器人会更新报名人数
5. 每人每天只能报名一次
6. 管理员可以使用 `/cancel` 命令取消当前报名

### 注意事项

- 确保机器人已添加到群组并具有管理员权限
- 报名信息会在群内实时更新
- 报名取消后，用户点击报名按钮会收到提示信息
- 所有消息都使用 HTML 格式，确保消息安全

## 技术栈

- Go 语言
- Telegram Bot API
- SQLite 数据库

## 许可证

MIT License 