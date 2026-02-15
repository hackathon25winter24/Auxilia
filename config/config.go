package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
    DBHost string
    DBUser string
    DBPass string
    DBName string
    DBPort string
    AppPort string
    AllowAllOrigins bool
}

func LoadConfig() (Config, error) {
    _ = godotenv.Load()
    cfg := Config{
        DBHost: os.Getenv("NS_MARIADB_HOSTNAME"),
        DBUser: os.Getenv("NS_MARIADB_USER"),
        DBPass: os.Getenv("NS_MARIADB_PASSWORD"),
        DBName: os.Getenv("NS_MARIADB_DATABASE"),
        DBPort: os.Getenv("NS_MARIADB_PORT"),
        AppPort: os.Getenv("PORT"),
    }
    if cfg.AppPort == "" {
        cfg.AppPort = "8080"
    }
    if cfg.DBHost == "" || cfg.DBPort == "" {
        return cfg, fmt.Errorf("missing DB config")
    }
    cfg.AllowAllOrigins = true
    return cfg, nil
}
