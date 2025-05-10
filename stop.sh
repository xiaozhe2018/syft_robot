#!/bin/bash

# 检查PID文件是否存在
if [ ! -f bot.pid ]; then
    echo "找不到PID文件，机器人可能未运行"
    exit 1
fi

# 读取PID
PID=$(cat bot.pid)

# 检查进程是否存在
if ! ps -p $PID > /dev/null; then
    echo "机器人进程不存在，可能已经停止"
    rm bot.pid
    exit 0
fi

# 停止进程
echo "正在停止机器人进程(PID: $PID)..."
kill $PID

# 等待进程结束
for i in {1..10}; do
    if ! ps -p $PID > /dev/null; then
        echo "机器人已停止"
        rm bot.pid
        exit 0
    fi
    sleep 1
done

# 如果进程仍然存在，强制终止
echo "机器人进程未能正常停止，正在强制终止..."
kill -9 $PID
rm bot.pid
echo "机器人已强制停止" 