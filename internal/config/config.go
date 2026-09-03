package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port              string
	MySQLDSN          string
	R2Endpoint        string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Bucket          string
	R2PublicURL       string
	MimoAPIKey        string
	MimoBaseURL       string
	MimoModel         string
	MimoTimeoutSec    int
	MimoInputCost     float64
	MimoOutputCost    float64
	WhatsAppVerify    string
	WhatsAppAppSecret string
	WhatsAppToken     string
	WhatsAppPhoneID   string
	WhatsAppGraphVer  string
	SignupURL         string
}

func Load() Config {
	return Config{
		Port:              value("APP_PORT", "8080"),
		MySQLDSN:          os.Getenv("MYSQL_DSN"),
		R2Endpoint:        os.Getenv("R2_ENDPOINT"),
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:          value("R2_BUCKET", "ai-smart-storage"),
		R2PublicURL:       os.Getenv("R2_PUBLIC_URL"),
		MimoAPIKey:        os.Getenv("MIMO_API_KEY"),
		MimoBaseURL:       value("MIMO_BASE_URL", "https://api.xiaomimimo.com/v1"),
		MimoModel:         value("MIMO_MODEL", "mimo-v2.5"),
		MimoTimeoutSec:    integer("MIMO_TIMEOUT_SECONDS", 120),
		MimoInputCost:     decimal("MIMO_INPUT_COST_PER_1K", 0),
		MimoOutputCost:    decimal("MIMO_OUTPUT_COST_PER_1K", 0),
		WhatsAppVerify:    os.Getenv("WHATSAPP_VERIFY_TOKEN"),
		WhatsAppAppSecret: os.Getenv("WHATSAPP_APP_SECRET"),
		WhatsAppToken:     os.Getenv("WHATSAPP_ACCESS_TOKEN"),
		WhatsAppPhoneID:   os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
		WhatsAppGraphVer:  value("WHATSAPP_GRAPH_VERSION", "v22.0"),
		SignupURL:         value("SIGNUP_URL", "http://localhost:8080/signup"),
	}
}

func decimal(key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func value(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func integer(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
