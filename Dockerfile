FROM golang:1.24-alpine AS builder

WORKDIR /app

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建
RUN CGO_ENABLED=0 GOOS=linux go build -o notify .

# 使用轻量级基础镜像
FROM alpine:latest

WORKDIR /app

# 安装 CA 证书，以支持 HTTPS 请求
RUN apk --no-cache add ca-certificates

# 从构建阶段复制二进制文件
COPY --from=builder /app/notify .
# 复制默认配置文件
COPY --from=builder /app/config/config.example.yaml /app/config/config.yaml

# 设置配置文件的挂载点
VOLUME /app/config

ENTRYPOINT ["/app/notify", "-c", "/app/config/config.yaml"] 