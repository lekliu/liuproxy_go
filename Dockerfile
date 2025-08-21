# --- STAGE 1: Build ---

# 使用官方的 Go Alpine 镜像作为构建环境，它更小
FROM golang:1.22-alpine AS builder
LABEL authors="lek liu"

ENTRYPOINT ["top", "-b"]

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum 以利用 Docker 的层缓存
COPY src/go.mod src/go.sum ./

RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct

RUN go mod download

# 复制所有源代码
COPY src/ .

# 构建应用
# -o 指定输出文件路径，我们将其放在 /app/bin/ 目录下，以匹配 main.go 中的相对路径
# CGO_ENABLED=0 和 -a 确保生成静态链接的可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -a -o /app/bin/liuproxy ./main

# --- STAGE 2: Production ---
# 使用一个极小的 Alpine 镜像作为最终镜像的基础
FROM alpine:latest

# 将配置文件复制到镜像中
# 我们创建 /app/conf 目录来存放它
COPY conf/config.ini /app/conf/config.ini

# 从构建阶段 (builder) 复制编译好的可执行文件
COPY --from=builder /app/bin/liuproxy /app/bin/liuproxy

# 暴露远端服务器需要监听的端口
# 这些端口号应与 config.ini [remote] 部分匹配
EXPOSE 10089
EXPOSE 10090

# 设置工作目录，这样 main.go 中的 "../../conf/config.ini" 就能正确找到文件
WORKDIR /app/bin

# 容器启动时运行的命令
CMD ["./liuproxy", "--config", "/app/conf/config.ini"]
