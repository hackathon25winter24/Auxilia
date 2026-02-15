package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost          string
	DBUser          string
	DBPass          string
	DBName          string
	DBPort          string
	AppPort         string
	AllowAllOrigins bool
}

func LoadConfig() (Config, error) {
	// .envがあれば読み込む。なければ無視して環境変数を直接見に行く
	_ = godotenv.Load()

	cfg := Config{
		// 取得して空ならデフォルト値をセットする関数（下で定義）を使う
		DBHost:  getEnv("NS_MARIADB_HOSTNAME", "127.0.0.1"),
		DBUser:  getEnv("NS_MARIADB_USER", "root"),
		DBPass:  getEnv("NS_MARIADB_PASSWORD", "password"), // Dockerのパスワード
		DBName:  getEnv("NS_MARIADB_DATABASE", "auxilia_db"),
		DBPort:  getEnv("NS_MARIADB_PORT", "3306"),
		AppPort: getEnv("PORT", "8080"),
		AllowAllOrigins: true,
	}

	// 必須チェック（本番環境での事故防止）
	// ただし、デフォルト値を入れているので基本的には通ります
	if cfg.DBHost == "" {
		return cfg, fmt.Errorf("DB_HOST is still empty")
	}

	return cfg, nil
}

// getEnv は環境変数を取得し、空だった場合は fallback を返します
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}