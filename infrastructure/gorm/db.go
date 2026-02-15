package gormdb

import (
	"fmt"
	"log"

	"auxilia/config"
	"auxilia/domain/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func NewGormDB(cfg config.Config) (*gorm.DB, error) {
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
        cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName)

    db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
    if err != nil {
        return nil, err
    }

    if err := db.AutoMigrate(&model.User{}); err != nil {
        log.Printf("AutoMigrate error: %v", err)
        return nil, err
    }

    return db, nil
}
