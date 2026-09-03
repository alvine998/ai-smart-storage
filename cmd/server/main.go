package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/config"
	"ai-smart-storage/internal/database"
	"ai-smart-storage/internal/diagnostics"
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

	// ── MySQL ──────────────────────────────────────────────────────
	start := time.Now()
	store, err := database.Open(cfg.MySQLDSN)
	if err != nil {
		log.Printf("[diagnostics] ✗ DB (MySQL)     FAILED error=%v latency=%dms dsn=%s", err, time.Since(start).Milliseconds(), maskDSN(cfg.MySQLDSN))
		log.Fatalf("mysql: %v", err)
	}
	log.Printf("[diagnostics] ✓ DB (MySQL)       connected (%s) latency=%dms", maskDSN(cfg.MySQLDSN), time.Since(start).Milliseconds())
	defer store.Close()

	// ── R2 ─────────────────────────────────────────────────────────
	r2Start := time.Now()
	r2, err := r2storage.New(r2storage.Config{
		Endpoint:        cfg.R2Endpoint,
		AccessKeyID:     cfg.R2AccessKeyID,
		SecretAccessKey: cfg.R2SecretAccessKey,
		Bucket:          cfg.R2Bucket,
		PublicURL:       cfg.R2PublicURL,
	})
	if err != nil {
		log.Printf("[diagnostics] ✗ Cloudflare R2    FAILED error=%v latency=%dms bucket=%s endpoint=%s", err, time.Since(r2Start).Milliseconds(), cfg.R2Bucket, cfg.R2Endpoint)
		log.Fatalf("r2: %v", err)
	}
	// R2 client is constructed synchronously; bucket reachability is verified
	// later in the unified diagnostics probe. Log construction latency.
	log.Printf("[diagnostics] ✓ Cloudflare R2    client ready (bucket=%s endpoint=%s) latency=%dms", cfg.R2Bucket, cfg.R2Endpoint, time.Since(r2Start).Milliseconds())

	aiClient := ai.NewClient(cfg.MimoAPIKey, cfg.MimoBaseURL, cfg.MimoModel, time.Duration(cfg.MimoTimeoutSec)*time.Second)
	// Token Plan is credit-based; dollar costs must be 0. Warn if mis-configured.
	if cfg.IsTokenPlan() && (cfg.MimoInputCost != 0 || cfg.MimoOutputCost != 0) {
		log.Printf("[diagnostics] ⚠ MiMo Token Plan detected (baseURL=%s key=%s…) but MIMO_INPUT/OUTPUT_COST_PER_1K are non-zero (%.5f/%.5f) — cost will be forced to 0; set them to 0 for token-plan to silence this warning", cfg.MimoBaseURL, maskTail(cfg.MimoAPIKey, 6), cfg.MimoInputCost, cfg.MimoOutputCost)
	}
	if cfg.IsTokenPlan() {
		log.Printf("[diagnostics] ℹ MiMo billing mode: Token Plan (credits: mimo-v2.5 2/100/200, mimo-v2.5-pro 2.5/300/600 per token) — dollar cost disabled")
	} else if aiClient.IsConfigured() {
		log.Printf("[diagnostics] ℹ MiMo billing mode: Pay-as-you-go (input $%.5f/1K output $%.5f/1K)", cfg.MimoInputCost, cfg.MimoOutputCost)
	}
	wa := whatsapp.New(cfg.WhatsAppToken, cfg.WhatsAppVerify, cfg.WhatsAppAppSecret, cfg.WhatsAppPhoneID, cfg.WhatsAppGraphVer)
	redisOptions, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Printf("[diagnostics] ✗ Redis            FAILED error=%v (invalid REDIS_URL=%s)", err, cfg.RedisURL)
		log.Fatalf("redis configuration: %v", err)
	}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	secret := cfg.JWTSecret
	if secret == "" {
		secret = randomSecret()
		log.Printf("warning: APP_JWT_SECRET is not set; using an ephemeral secret, issued tokens stop working after restart")
	}

	// ── Unified startup diagnostics (ping every dependency) ────────
	diagCtx, diagCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer diagCancel()
	diag := diagnostics.New(cfg, store, r2, aiClient, wa, redisClient)
	diag.LogStartup(diagCtx)
	app := fiber.New(fiber.Config{
		AppName:   "AI Smart Storage",
		BodyLimit: 500 * 1024 * 1024, // 500MB
	})
	app.Use(middleware.CORS(cfg.CORSAllowedOrigins))
	health.NewWithDiagnostics(store, r2, redisClient, aiClient, wa, cfg).Register(app)
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

func maskDSN(dsn string) string {
	if dsn == "" {
		return "(empty)"
	}
	at := strings.Index(dsn, "@")
	colon := strings.Index(dsn, ":")
	if colon != -1 && at != -1 && colon < at {
		return dsn[:colon] + ":***" + dsn[at:]
	}
	if at != -1 {
		return "***" + dsn[at:]
	}
	return "***"
}

func maskTail(s string, keep int) string {
	if s == "" {
		return "(empty)"
	}
	if len(s) <= keep {
		return s
	}
	return strings.Repeat("*", len(s)-keep) + s[len(s)-keep:]
}
