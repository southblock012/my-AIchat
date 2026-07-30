#============= 阶段1：编译 ===========
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

# 使用国内镜像，避免默认 proxy.golang.org 被墙导致构建失败
ENV GOPROXY=https://goproxy.cn,direct

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o main .

# ============ 阶段2：运行 ============
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]