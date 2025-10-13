# 多阶段构建 - 编译阶段
FROM golang:1.17 AS builder

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /app

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译可执行文件
RUN go build -o transfer .

# 运行阶段
FROM alpine:latest

# 设置时区
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从编译阶段复制可执行文件
COPY --from=builder /app/transfer .

# 创建配置文件目录和日志目录
RUN mkdir -p /app/config /app/store/log /app/store/db

# 设置可执行权限
RUN chmod +x transfer

# 暴露端口
EXPOSE 8060

# 启动命令
ENTRYPOINT ["./transfer"]