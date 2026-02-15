package config

import (
	"os"
	"testing"
)

func TestLoadConfigValid(t *testing.T) {
	// Set valid env vars
	os.Setenv("NS_MARIADB_HOSTNAME", "localhost")
	os.Setenv("NS_MARIADB_PORT", "3306")
	os.Setenv("NS_MARIADB_USER", "user")
	os.Setenv("NS_MARIADB_PASSWORD", "pass")
	os.Setenv("NS_MARIADB_DATABASE", "testdb")
	os.Setenv("PORT", "8000")
	defer func() {
		os.Unsetenv("NS_MARIADB_HOSTNAME")
		os.Unsetenv("NS_MARIADB_PORT")
		os.Unsetenv("NS_MARIADB_USER")
		os.Unsetenv("NS_MARIADB_PASSWORD")
		os.Unsetenv("NS_MARIADB_DATABASE")
		os.Unsetenv("PORT")
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.DBHost != "localhost" {
		t.Errorf("Expected DBHost 'localhost', got '%s'", cfg.DBHost)
	}
	if cfg.DBPort != "3306" {
		t.Errorf("Expected DBPort '3306', got '%s'", cfg.DBPort)
	}
	if cfg.AppPort != "8000" {
		t.Errorf("Expected AppPort '8000', got '%s'", cfg.AppPort)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Save and then clear env vars
	savedHost := os.Getenv("NS_MARIADB_HOSTNAME")
	savedPort := os.Getenv("NS_MARIADB_PORT")
	os.Unsetenv("NS_MARIADB_HOSTNAME")
	os.Unsetenv("NS_MARIADB_PORT")
	os.Unsetenv("PORT")
	defer func() {
		if savedHost != "" {
			os.Setenv("NS_MARIADB_HOSTNAME", savedHost)
		}
		if savedPort != "" {
			os.Setenv("NS_MARIADB_PORT", savedPort)
		}
	}()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Check defaults are applied
	if cfg.DBHost == "" {
		t.Error("Expected DBHost to have default value")
	}
	if cfg.DBPort == "" {
		t.Error("Expected DBPort to have default value")
	}
	if cfg.AppPort != "8080" {
		t.Errorf("Expected default AppPort '8080', got '%s'", cfg.AppPort)
	}
}

