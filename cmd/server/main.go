package main

import (
	"log"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/config"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/chat"
	"ai-smart-storage/internal/http/health"
	"ai-smart-storage/internal/http/middleware"
	"ai-smart-storage/internal/http/packages"
	"ai-smart-storage/internal/http/storage"
	"ai-smart-storage/internal/http/userpackages"
	"ai-smart-storage/internal/http/users"
	whatsapphttp "ai-smart-storage/internal/http/whatsapp"
	r2storage "ai-smart-storage/internal/storage"
	"ai-smart-storage/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	store, err := database.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer store.Close()
	r2, err := r2storage.New(r2storage.Config{
		Endpoint:        cfg.R2Endpoint,
		AccessKeyID:     cfg.R2AccessKeyID,
		SecretAccessKey: cfg.R2SecretAccessKey,
		Bucket:          cfg.R2Bucket,
		PublicURL:       cfg.R2PublicURL,
	})
	if err != nil {
		log.Fatalf("r2: %v", err)
	}
	aiClient := ai.NewClient(cfg.MimoAPIKey, cfg.MimoBaseURL, cfg.MimoModel, time.Duration(cfg.MimoTimeoutSec)*time.Second)
	wa := whatsapp.New(cfg.WhatsAppToken, cfg.WhatsAppAppSecret, cfg.WhatsAppPhoneID, cfg.WhatsAppGraphVer)
	app := fiber.New(fiber.Config{AppName: "AI Smart Storage"})
	app.Use(middleware.OpenCORS())
	health.New().Register(app)
	users.New(store).Register(app)
	packages.New(store).Register(app)
	userpackages.New(store).Register(app)
	storage.New(r2, store).Register(app)
	chat.New(aiClient, store).Register(app)
	whatsapphttp.New(aiClient, store, wa).Register(app)
	log.Printf("API listening on :%s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}
