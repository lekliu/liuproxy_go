#!/bin/bash

# 定义程序的名称
PROGRAM="liuproxy-linux"
PIDFILE="/tmp/${PROGRAM}-r.pid"

chmod +x ./"$PROGRAM"

# 检查程序是否已经在运行
if [ -f "$PIDFILE" ]; then
    if ps -p "$(cat "$PIDFILE")" > /dev/null; then
        echo "$PROGRAM is already running."
        exit 1
    else
        # 如果 PID 文件存在但进程不存在，删除 PID 文件
        rm "$PIDFILE"
    fi
fi

# 循环启动程序
while true; do
    # 启动程序并将其放入后台
    ./"$PROGRAM" -r &

    # 将程序的 PID 写入 PID 文件
    echo $! > "$PIDFILE"

    # 等待程序退出
    wait $!

    # 当程序异常退出时，重新启动
    echo "$PROGRAM has exited. Restarting..."

    # 等待一段时间后再重启，以避免快速重启导致资源耗尽
    sleep 2
done

# nohup ./remote.sh > remote.log 2>&1
