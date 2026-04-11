package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/listener"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
)

// Build-time variables set by -ldflags
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

const (
	defaultConfigPath = "/etc/packetpony/config.yaml"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", defaultConfigPath, "path to configuration file")
	showVersion := flag.Bool("version", false, "show version and exit")
	flag.Parse()

	// Show version and exit
	if *showVersion {
		fmt.Printf("PacketPony %s\n", version)
		fmt.Printf("  Commit:     %s\n", commit)
		fmt.Printf("  Build time: %s\n", buildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logging early so we can use it for all subsequent messages
	logger, err := logging.NewMultiLogger(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	// Log startup messages via logger (not fmt.Printf)
	logger.LogInfo("PacketPony starting", map[string]interface{}{
		"version": version,
		"commit":  commit,
		"server":  cfg.Server.Name,
		"config":  *configPath,
	})

	// Setup metrics
	proxyMetrics := metrics.NewProxyMetrics()
	if err := metrics.StartMetricsServer(cfg.Metrics.Prometheus, logger); err != nil {
		logger.LogError("Failed to start metrics server", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	if cfg.Metrics.Prometheus.Enabled {
		logger.LogInfo("Prometheus metrics server started", map[string]interface{}{
			"address": cfg.Metrics.Prometheus.ListenAddress,
			"path":    cfg.Metrics.Prometheus.Path,
		})
	}

	// Create listener manager
	manager, err := listener.NewManager(cfg, logger, proxyMetrics)
	if err != nil {
		logger.LogError("Failed to create listener manager", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Start all listeners
	if err := manager.Start(); err != nil {
		logger.LogError("Failed to start listeners", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.LogInfo("PacketPony is running", map[string]interface{}{
		"listeners": len(cfg.Listeners),
	})

	// Wait for shutdown signal
	sig := <-sigChan
	logger.LogInfo("Received shutdown signal", map[string]interface{}{
		"signal": sig.String(),
	})

	// Graceful shutdown with configured timeout
	if err := manager.GracefulShutdown(cfg.Server.ShutdownTimeout); err != nil {
		logger.LogError("Error during graceful shutdown", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	logger.LogInfo("PacketPony stopped gracefully", nil)
}
