package main

import (
	"context"
	"embed"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

//go:embed web
var webFS embed.FS

func init() {
	// 在此处设置日志前缀以便调试
	log.SetPrefix("[ant2oa] ")
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	// Attempt to load .env file
	_ = godotenv.Load()

	installFlag := flag.Bool("install", false, "Install as systemd service (Linux only)")
	flag.Parse()

	if *installFlag {
		installService()
		return
	}

	// Load configurations
	if err := loadAPIKeys(); err != nil {
		log.Printf("Warning: Failed to load keys.json: %v", err)
	}
	if err := loadModelRoutes(); err != nil {
		log.Printf("Warning: Failed to load routes.json: %v", err)
	}

	// ================= Rate Limiter Setup =================
	rpmStr := os.Getenv("RATE_LIMIT")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Catch-all for main exit

	if rpmStr != "" {
		rpm, err := strconv.Atoi(rpmStr)
		if err == nil && rpm > 0 {
			rateLimitEnabled = true
			burst := 5 // Default burst
			if rpm < 5 {
				burst = rpm
			}
			limiter = make(chan struct{}, burst)

			// Initial fill
			for i := 0; i < burst; i++ {
				limiter <- struct{}{}
			}

			interval := time.Minute / time.Duration(rpm)
			go func(ctx context.Context) {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						select {
						case limiter <- struct{}{}:
						default:
						}
					case <-ctx.Done():
						return
					}
				}
			}(ctx)
			log.Printf("Rate Limit Enabled: %d RPM", rpm)
		} else {
			log.Printf("Warning: Invalid RATE_LIMIT '%s' (expected >0 int). Rate limiting disabled.", rpmStr)
		}
	} else {
		log.Println("Rate Limit: Unlimited (set RATE_LIMIT env var to enable)")
	}

	// ================= Server Setup =================
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}

	base := os.Getenv("OPENAI_BASE_URL")
	if base == "" {
		log.Fatal("OPENAI_BASE_URL environment variable is required")
	}
	model := os.Getenv("OPENAI_MODEL")

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/v1/messages", messagesHandler(base, model))
	mux.HandleFunc("/v1/complete", completeHandler(base, model))
	mux.HandleFunc("/v1/models", modelsHandler(base))
	mux.HandleFunc("/health", enhancedHealthHandler(base))

	// Metrics routes
	mux.HandleFunc("/metrics", metricsHandler())
	mux.HandleFunc("/metrics/json", metricsJSONHandler())

	// Web UI routes
	mux.HandleFunc("/config", configWebHandler)

	// Config API
	mux.HandleFunc("/api/config", configHandler)

	// Get max request size from env (default 10MB)
	maxRequestSize := int64(10 * 1024 * 1024)
	if maxSizeStr := os.Getenv("MAX_REQUEST_SIZE"); maxSizeStr != "" {
		if size, err := strconv.ParseInt(maxSizeStr, 10, 64); err == nil && size > 0 {
			maxRequestSize = size
		}
	}

	// Apply middleware chain
	handler := chainMiddleware(
		mux,
		loggingMiddleware,
		corsMiddleware,
		apiKeyAuthMiddleware,
		maxBytesMiddleware(maxRequestSize),
	)

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Listening on", ln.Addr())
	log.Printf("Max request size: %d bytes", maxRequestSize)

	srv := &http.Server{Handler: handler}

	// Run Server in Goroutine
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Listen Error: %s", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctxGrace, cancelGrace := context.WithTimeout(context.Background(), 10*time.Second) // 10s grace period
	defer cancelGrace()

	if err := srv.Shutdown(ctxGrace); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server shutdown gracefully")
	}

	log.Println("Server exited cleanly")
}
