package config

import (
	"fmt"
	"os"
	"time"
)

// Config はアプリケーション全体の設定を保持します
type Config struct {
	// データベース設定
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// サーバー設定
	Port                 string
	JWTSecretKey         string
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration

	// セキュリティ設定
	PasswordMinLength int
	MaxLoginAttempts  int
	LockoutDuration   time.Duration
}

// LoadConfig は環境変数から設定を読み込みます
func LoadConfig() *Config {
	config := &Config{
		// Database
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "postgres"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "auxilia"),
		DBSSLMode:  getEnv("DB_SSLMODE", "require"), // 本番環境では常にrequireに

		// Server
		Port:                 getEnv("PORT", "8080"),
		JWTSecretKey:         getEnv("JWT_SECRET_KEY", ""), // 本番環境では必須
		AccessTokenDuration:  15 * time.Minute,             // アクセストークンは短期（15分）
		RefreshTokenDuration: 7 * 24 * time.Hour,           // リフレッシュトークンは長期（7日）

		// Security
		PasswordMinLength: 8,
		MaxLoginAttempts:  5,
		LockoutDuration:   15 * time.Minute,
	}

	// 重要な設定の検証
	if config.JWTSecretKey == "" {
		fmt.Println("警告: JWT_SECRET_KEYが設定されていません。開発環境でもセキュアなキーを設定してください。")
	}

	return config
}

// getEnv は環境変数を取得し、ない場合はデフォルト値を返します
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// GetDSN はPostgreSQLの接続文字列を生成します
func (c *Config) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode,
	)
}
