# --- STAGE 1: Build Stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /app

# 复制模块文件并下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制所有源代码
COPY . .

# 分别编译 local 和 remote 两个可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/local_server ./cmd/local
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/remote_server ./cmd/remote


# --- STAGE 2: Remote Image ---
FROM alpine:latest AS remote
WORKDIR /app
# 只复制 remote 的可执行文件和配置文件
COPY --from=builder /app/bin/remote_server .
COPY configs/remote.ini ./configs/remote.ini
EXPOSE 10089
# 启动命令指向新的可执行文件和配置文件路径
CMD ["./remote_server", "--config", "configs/remote.ini"]


# --- STAGE 3: Local Image ---
FROM alpine:latest AS local
WORKDIR /app
# 只复制 local 的可执行文件和配置文件
COPY --from=builder /app/bin/local_server .
COPY configs/local.ini ./configs/local.ini
EXPOSE 8080 9088
CMD ["./local_server", "--config", "configs/local.ini"]