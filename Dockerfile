# --- STAGE 1: Build Stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 复制模块文件并下载依赖
COPY go.mod go.sum ./

RUN export GOPROXY=https://goproxy.cn,direct && go mod download

# 复制所有源代码
COPY . .

# 只编译 local 可执行文件
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/liuproxy-go ./cmd/local


# --- STAGE 2: Final Image ---
FROM alpine:latest

WORKDIR /app

# 只从builder阶段复制编译好的二进制文件和默认配置文件
COPY --from=builder /app/bin/liuproxy-go .
COPY configs/ ./configs/

# 暴露服务端口
EXPOSE 8081 9088

# 设置容器的启动命令
CMD ["./liuproxy-go", "--configdir", "configs"]