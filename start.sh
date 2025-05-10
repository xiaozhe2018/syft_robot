#!/bin/bash

# 编译项目
go build -o bot api/dinner.go

# 检查Redis是否运行
redis-cli ping > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "Redis未运行，正在启动Redis..."
    # 根据您的系统调整此命令
    sudo systemctl start redis
    sleep 2
fi

# 使用nohup后台运行Bot
nohup ./bot > bot.log 2>&1 &

# 保存进程ID
echo $! > bot.pid

echo "机器人已在后台启动，PID: $(cat bot.pid)"
echo "日志文件: $(pwd)/bot.log" 