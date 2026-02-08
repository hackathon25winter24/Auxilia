package db

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

var (
	db   *sql.DB
	once sync.Once
)

// InitDB はPostgreSQLへの接続を初期化します
func InitDB(dsn string) (*sql.DB, error) {
	var err error
	once.Do(func() {
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			log.Printf("Failed to open database: %v", err)
			return
		}

		// 接続プールの設定
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		// 接続テスト
		err = db.Ping()
		if err != nil {
			log.Printf("Failed to ping database: %v", err)
			return
		}

		log.Println("Database connected successfully")
	})

	return db, err
}

// GetDB はグローバルなDBコネクションを取得します
func GetDB() *sql.DB {
	return db
}

// CreateTables はデータベーステーブルを作成します
func CreateTables() error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	schema := `
	-- ユーザーテーブル
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) UNIQUE NOT NULL,
		username VARCHAR(50) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		is_deleted BOOLEAN DEFAULT FALSE
	);

	-- リフレッシュトークンテーブル（トークンの無効化管理用）
	CREATE TABLE IF NOT EXISTS refresh_tokens (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash VARCHAR(255) UNIQUE NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		is_revoked BOOLEAN DEFAULT FALSE,
		revoked_at TIMESTAMP
	);

	-- ログイン試行トレーサー（ブルートフォース攻撃対策）
	CREATE TABLE IF NOT EXISTS login_attempts (
		id SERIAL PRIMARY KEY,
		email VARCHAR(255) NOT NULL,
		ip_address VARCHAR(45) NOT NULL,
		success BOOLEAN NOT NULL,
		attempted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- インデックス作成
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
	CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);
	CREATE INDEX IF NOT EXISTS idx_login_attempts_email ON login_attempts(email);
	CREATE INDEX IF NOT EXISTS idx_login_attempts_attempted_at ON login_attempts(attempted_at);
	`

	_, err := db.Exec(schema)
	return err
}

// CloseDB はデータベース接続を閉じます
func CloseDB() error {
	if db != nil {
		return db.Close()
	}
	return nil
}
