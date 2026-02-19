package gorm

import (
    "fmt"
    "time"
    "auxilia/config"
    "auxilia/domain/model"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
)

func NewGormDB(cfg config.Config) (*gorm.DB, error) {
    var db *gorm.DB
    var err error
    dsn := cfg.DSN() // Config 側のメソッドを呼び出すだけ

    // リトライ処理（Docker起動時の安定性のために重要）
    for i := 0; i < 5; i++ {
        db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
        if err == nil {
            break
        }
        fmt.Printf("Attempt %d: Failed to connect to DB, retrying...\n", i+1)
        time.Sleep(3 * time.Second)
    }

    if err != nil {
        return nil, fmt.Errorf("could not connect to DB after retries: %w", err)
    }

    // マイグレーション
    if err := db.AutoMigrate(&model.User{}); err != nil {
        return nil, fmt.Errorf("migration failed: %w", err)
    }

    return db, nil
}