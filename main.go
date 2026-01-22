package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

//go:embed web
var webFS embed.FS

// Global Configuration with Thread Safety
type ServerConfig struct {
	sync.RWMutex
	BaseURL   string
	Model     string
	RateLimit int // RPM, 0 = unlimited
	APIKey    string
}

var (
	config     ServerConfig
	limiter    chan struct{} // Global limiter channel
	limiterStop chan struct{} // Channel to stop the rate limiter goroutine
	limiterMu  sync.RWMutex   // Protects limiter access
)

// Update global config and reset rate limiter if needed
func updateConfig(newBase, newModel, newKey string, newRate int) {
	config.Lock()
	defer config.Unlock()

	config.BaseURL = newBase
	config.Model = newModel
	config.APIKey = newKey

	// Reset rate limiter if changed
	if newRate != config.RateLimit {
		config.RateLimit = newRate
		setupRateLimiter(newRate)
	}
}

func setupRateLimiter(rpm int) {
	limiterMu.Lock()
	defer limiterMu.Unlock()

	// Stop the old rate limiter goroutine if running
	if limiterStop != nil {
		close(limiterStop)
		limiterStop = nil
	}

	if rpm <= 0 {
		limiter = nil
		limiterStop = nil
		log.Println("Rate Limit: Unlimited")
		return
	}

	burst := 5
	if rpm < 5 {
		burst = rpm
	}
	limiter = make(chan struct{}, burst)

	// Initial fill
	for i := 0; i < burst; i++ {
		limiter <- struct{}{}
	}

	// Create stop channel for the new goroutine
	limiterStop = make(chan struct{})
	interval := time.Minute / time.Duration(rpm)

	go func(ch chan struct{}, stopCh chan struct{}) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Non-blocking send
				select {
				case ch <- struct{}{}:
				default:
				}
			case <-stopCh:
				return
			}
		}
	}(limiter, limiterStop)

	log.Printf("Rate Limit: %d RPM", rpm)
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	// 1. Parse Flags
	installFlag := flag.Bool("install", false, "Install as systemd service (Linux only)")
	logPath := flag.String("log", "", "Log file path (default: stdout)")
	flag.Parse()

	if *installFlag {
		installService()
		return
	}

	// 2. Setup Logging
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("Error opening log file: %v", err)
		}
		defer f.Close()
		log.SetOutput(io.MultiWriter(os.Stdout, f)) // Write to both for visibility
		log.Printf("Logging to %s", *logPath)
	}

	// 3. Load Environment
	_ = godotenv.Load()

	// 4. Initialize Config
	// NOTE: We do not init 'limiter' here directly, updateConfig does it.
	// But we need to load initial values.

	// Helper to parse int env
	parseEnvInt := func(key string) int {

		// using naive approach
		var val int
		fmt.Sscanf(os.Getenv(key), "%d", &val)
		return val
	}

	initialRPM := parseEnvInt("RATE_LIMIT")

	// Set initial state
	apiKey := os.Getenv("OPENAI_API_KEY")
	updateConfig(
		os.Getenv("OPENAI_BASE_URL"),
		os.Getenv("OPENAI_MODEL"),
		apiKey,
		initialRPM,
	)
	// Log API key masked for security
	if apiKey != "" {
		log.Println("API Key: *****" + apiKey[len(apiKey)-4:])
	}

	// Validation
	config.RLock()
	if config.BaseURL == "" {
		log.Fatal("OPENAI_BASE_URL environment variable is required")
	}
	config.RUnlock()

	// 5. Server Setup
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/v1/messages", messagesHandler)
	mux.HandleFunc("/v1/complete", completeHandler)
	mux.HandleFunc("/v1/models", modelsHandler)
	mux.HandleFunc("/health", healthHandler())

	// Configuration & Testing
	mux.HandleFunc("/api/config", configHandler)
	mux.HandleFunc("/api/test", testConnectionHandler)

	// Web UI
	mux.HandleFunc("/config", configWebHandler)

	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Listening on", ln.Addr())

	// Run Server
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exited cleanly")
}
