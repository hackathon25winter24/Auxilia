# ビルドステージ
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN apk add --no-cache git
RUN go mod download
# プロジェクトルートの main.go をビルド
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# 実行ステージ
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
# もし.envを使いたい場合はコピー（環境変数で渡すなら不要）
# COPY --from=builder /app/.env . 

EXPOSE 8080
CMD ["./main"]