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

	// ================= Rate Limiter Setup =================
	rpmStr := os.Getenv("RATE_LIMIT")
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
			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for range ticker.C {
					select {
					case limiter <- struct{}{}:
					default:
					}
				}
			}()
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
	mux.HandleFunc("/health", healthHandler())

	// Web UI routes
	mux.HandleFunc("/config", configWebHandler)

	// Config API
	mux.HandleFunc("/api/config", configHandler)

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Listening on", ln.Addr())

	srv := &http.Server{Handler: mux}

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // 10s grace period
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	} else {
		log.Println("Server shutdown gracefully")
	}

	log.Println("Server exited cleanly")
}
