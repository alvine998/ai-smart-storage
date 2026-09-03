package diagnostics

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"ai-smart-storage/internal/ai"
	"ai-smart-storage/internal/config"
	"ai-smart-storage/internal/database"
	r2storage "ai-smart-storage/internal/storage"
	"ai-smart-storage/internal/whatsapp"

	"github.com/redis/go-redis/v9"
)

// Result holds a single dependency check outcome.
type Result struct {
	Name      string        `json:"name"`
	Status    string        `json:"status"` // "ok" | "error" | "skipped"
	Latency   time.Duration `json:"-"`               // internal duration for logging
	LatencyMs int64         `json:"latency_ms"`      // milliseconds exposed via JSON
	Detail    string        `json:"detail,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// Checker runs startup + health probes for all external dependencies.
type Checker struct {
	cfg   config.Config
	store *database.Store
	r2    *r2storage.Store
	ai    *ai.Client
	wa    *whatsapp.Service
	redis *redis.Client
}

// New creates a diagnostics checker from already-initialised clients.
// Any nil argument will be reported as "skipped".
func New(cfg config.Config, store *database.Store, r2 *r2storage.Store, aiClient *ai.Client, wa *whatsapp.Service, redisClient *redis.Client) *Checker {
	return &Checker{cfg: cfg, store: store, r2: r2, ai: aiClient, wa: wa, redis: redisClient}
}

// CheckAll runs every probe concurrently (each with its own timeout) and
// returns ordered results: DB, Redis, R2, WhatsApp, MiMo.
func (c *Checker) CheckAll(ctx context.Context) []Result {
	type job struct {
		name string
		fn   func(context.Context) (string, error)
	}
	jobs := []job{
		{name: "DB (MySQL)", fn: c.checkDB},
		{name: "Redis", fn: c.checkRedis},
		{name: "Cloudflare R2", fn: c.checkR2},
		{name: "WhatsApp API", fn: c.checkWhatsApp},
		{name: "MiMo V2.5", fn: c.checkMiMo},
	}

	results := make([]Result, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func(idx int, name string, fn func(context.Context) (string, error)) {
			defer wg.Done()
			start := time.Now()
			subCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
			defer cancel()
			detail, err := fn(subCtx)
			latency := time.Since(start)
			r := Result{Name: name, Latency: latency, LatencyMs: latency.Milliseconds()}
			if err != nil {
				// Distinguish "not configured" from real connectivity errors
				if isSkipped(err) {
					r.Status = "skipped"
					r.Detail = detail
					if detail == "" {
						r.Detail = err.Error()
					}
				} else {
					r.Status = "error"
					r.Error = err.Error()
					if detail != "" {
						r.Detail = detail
					}
				}
			} else {
				r.Status = "ok"
				r.Detail = detail
			}
			results[idx] = r
		}(i, j.name, j.fn)
	}
	wg.Wait()
	return results
}

// LogStartup prints a coloured/structured boot report via the std logger.
// Intended to be called once from main() right after all clients are constructed.
func (c *Checker) LogStartup(ctx context.Context) []Result {
	log.Println("──────────────────────────────────────────────────")
	log.Println("[diagnostics] probing external dependencies...")
	results := c.CheckAll(ctx)
	allOK := true
	for _, r := range results {
		switch r.Status {
		case "ok":
			log.Printf("[diagnostics] ✓ %-16s connected (%s) latency=%dms", r.Name, r.Detail, r.LatencyMs)
		case "skipped":
			log.Printf("[diagnostics] ⊘ %-16s skipped  (%s) latency=%dms", r.Name, r.Detail, r.LatencyMs)
			// skipped is not fatal but worth surfacing
		case "error":
			allOK = false
			if r.Detail != "" {
				log.Printf("[diagnostics] ✗ %-16s FAILED (%s) error=%s latency=%dms", r.Name, r.Detail, r.Error, r.LatencyMs)
			} else {
				log.Printf("[diagnostics] ✗ %-16s FAILED error=%s latency=%dms", r.Name, r.Error, r.LatencyMs)
			}
		}
	}
	if allOK {
		log.Println("[diagnostics] all dependencies OK")
	} else {
		log.Println("[diagnostics] one or more dependencies FAILED — see lines above (service will still start; /health will report degraded)")
	}
	log.Println("──────────────────────────────────────────────────")
	return results
}

// OverallStatus derives an aggregate status string from results.
func OverallStatus(results []Result) string {
	for _, r := range results {
		if r.Status == "error" {
			return "degraded"
		}
	}
	return "ok"
}

// ---- individual probes ----

func (c *Checker) checkDB(ctx context.Context) (string, error) {
	if c.store == nil {
		return "store not initialized", fmt.Errorf("skipped: store not initialized")
	}
	if strings.TrimSpace(c.cfg.MySQLDSN) == "" {
		return "MYSQL_DSN empty", fmt.Errorf("skipped: MYSQL_DSN not set")
	}
	if err := c.store.Ping(ctx); err != nil {
		return maskDSN(c.cfg.MySQLDSN), err
	}
	return maskDSN(c.cfg.MySQLDSN), nil
}

func (c *Checker) checkRedis(ctx context.Context) (string, error) {
	if c.redis == nil {
		return "client not initialized", fmt.Errorf("skipped: redis client not initialized")
	}
	detail := c.cfg.RedisURL
	if u, err := url.Parse(c.cfg.RedisURL); err == nil && u.Host != "" {
		detail = u.Host
		if u.Path != "" && u.Path != "/" {
			detail += u.Path
		}
	}
	if err := c.redis.Ping(ctx).Err(); err != nil {
		return detail, err
	}
	return detail, nil
}

func (c *Checker) checkR2(ctx context.Context) (string, error) {
	if c.r2 == nil {
		return "client not initialized", fmt.Errorf("skipped: R2 client not initialized")
	}
	detail := fmt.Sprintf("bucket=%s endpoint=%s", c.cfg.R2Bucket, c.cfg.R2Endpoint)
	if c.cfg.R2Endpoint == "" || c.cfg.R2AccessKeyID == "" || c.cfg.R2SecretAccessKey == "" {
		return detail, fmt.Errorf("skipped: R2_ENDPOINT / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY not fully set")
	}
	if err := c.r2.Ping(ctx); err != nil {
		return detail, err
	}
	return detail, nil
}

func (c *Checker) checkWhatsApp(ctx context.Context) (string, error) {
	if c.wa == nil {
		return "client not initialized", fmt.Errorf("skipped: whatsapp client not initialized")
	}
	detail := fmt.Sprintf("phoneID=%s graph=%s", maskTail(c.cfg.WhatsAppPhoneID, 4), c.cfg.WhatsAppGraphVer)
	if !c.wa.IsConfigured() {
		return detail, fmt.Errorf("skipped: WHATSAPP_ACCESS_TOKEN or WHATSAPP_PHONE_NUMBER_ID not set")
	}
	if err := c.wa.Ping(ctx); err != nil {
		return detail, err
	}
	return detail, nil
}

func (c *Checker) checkMiMo(ctx context.Context) (string, error) {
	if c.ai == nil {
		return "client not initialized", fmt.Errorf("skipped: mimo client not initialized")
	}
	mode := "Pay-as-you-go"
	if c.ai.IsTokenPlan() || c.cfg.IsTokenPlan() {
		mode = "Token Plan"
	}
	detail := fmt.Sprintf("mode=%s model=%s baseURL=%s", mode, c.cfg.MimoModel, c.cfg.MimoBaseURL)
	if !c.ai.IsConfigured() {
		return detail, fmt.Errorf("skipped: MIMO_API_KEY not set")
	}
	if err := c.ai.Ping(ctx); err != nil {
		// Surface token-plan hint when the API key looks valid but server rejects it:
		// helps distinguish quota/expired vs mis-configured payg key.
		if c.ai.IsTokenPlan() && strings.Contains(err.Error(), "Invalid API Key") {
			return detail, fmt.Errorf("%w (hint: Token Plan key %q rejected — check cluster matches your subscription (sgp/cn/ams) and that the plan is still active)", err, maskTail(c.cfg.MimoAPIKey, 6))
		}
		return detail, err
	}
	return detail, nil
}

// ---- helpers ----

func isSkipped(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "skipped:")
}

func maskDSN(dsn string) string {
	// dsn form: user:pass@tcp(host:port)/db?params
	// Never log the password in clear text.
	if dsn == "" {
		return ""
	}
	at := strings.Index(dsn, "@")
	colon := strings.Index(dsn, ":")
	if colon != -1 && at != -1 && colon < at {
		user := dsn[:colon]
		return user + ":***" + dsn[at:]
	}
	// fallback: just show tail host part
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
