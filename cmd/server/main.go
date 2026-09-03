package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/config"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/http/ai_logs"
	authhttp "ai-smart-storage/internal/http/auth"
	"ai-smart-storage/internal/http/business"
	"ai-smart-storage/internal/http/chat"
	"ai-smart-storage/internal/http/documents"
	"ai-smart-storage/internal/http/health"
	"ai-smart-storage/internal/http/invoices"
	"ai-smart-storage/internal/http/middleware"
	"ai-smart-storage/internal/http/plans"
	"ai-smart-storage/internal/http/storage"
	"ai-smart-storage/internal/http/subscriptions"
	"ai-smart-storage/internal/http/usagequota"
	"ai-smart-storage/internal/http/users"
	whatsapphttp "ai-smart-storage/internal/http/whatsapp"
	r2storage "ai-smart-storage/internal/storage"
	"ai-smart-storage/internal/whatsapp"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
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
	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis configuration: %v", err)
	}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	secret := cfg.JWTSecret
	if secret == "" {
		secret = randomSecret()
		log.Printf("warning: APP_JWT_SECRET is not set; using an ephemeral secret, issued tokens stop working after restart")
	}
	app := fiber.New(fiber.Config{
		AppName:   "AI Smart Storage",
		BodyLimit: 500 * 1024 * 1024, // 500MB
	})
	app.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	health.New(store, r2).Register(app)
	authhttp.New(store, secret, time.Duration(cfg.JWTTTLHours)*time.Hour).Register(app)
	users.New(store).RegisterPublic(app)
	whatsappHandler := whatsapphttp.New(aiClient, store, wa, cfg.SignupURL, redisClient)
	whatsappHandler.Register(app)
	api := app.Group("/", middleware.RequireAuth(secret))
	users.New(store).Register(api)
	usagequota.New(store).Register(api)
	ai_logs.New(store).Register(api)
	business.New(store).Register(api)
	documents.New(store, r2).Register(api)
	plans.New(store).Register(api)
	subscriptions.New(store).Register(api)
	invoices.New(store).Register(api)
	storage.NewWithDB(r2, store).Register(api)
	chat.New(aiClient, store, cfg.MimoInputCost, cfg.MimoOutputCost).Register(api)
	whatsappHandler.RegisterProtected(api)
	log.Printf("API listening on :%s", cfg.Port)

	// Set up graceful shutdown
	go func() {
		sigch := make(chan os.Signal, 1)
		signal.Notify(sigch, syscall.SIGTERM, syscall.SIGINT)
		<-sigch
		log.Println("shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Fatal(app.Listen(":" + cfg.Port))
}

func randomSecret() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("generate jwt secret: %v", err)
	}
	return hex.EncodeToString(buf)
}
