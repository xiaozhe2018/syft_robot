# 深夜饭堂机器人

一个基于 Telegram 的晚餐报名机器人，使用 go-zero 框架开发。

## 功能特点

- 支持群组内发起晚餐报名
- 支持用户报名参加
- 每人每天只能报名一次
- 报名信息实时更新
- 支持取消报名（仅发起人可用）

## 安装依赖

```bash
go mod tidy
```

## 配置

1. 复制 `etc/dinner.yaml.example` 到 `etc/dinner.yaml`
2. 修改配置文件中的 Bot Token 和 Redis 配置

## 运行

```bash
go run api/dinner.go -f etc/dinner.yaml
```

## 命令列表

- `/start` - 开始使用机器人
- `/help` - 显示帮助信息
- `/dinner` - 开始今天的晚餐报名
- `/cancel` - 取消当前报名（仅发起人可用）

## 技术栈

- Go
- go-zero
- Redis
- Telegram Bot API

## 许可证

MIT License 