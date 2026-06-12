package main

import (
	"flag"
	"fmt"
	"os"

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
	configDir := flag.String("config-dir", "", "listener fragment directory (default: config.d next to the main config)")
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
	loadConfig := func() (*config.Config, error) {
		if *configDir != "" {
			return config.LoadConfigWithDir(*configPath, *configDir)
		}
		return config.LoadConfig(*configPath)
	}

	cfg, err := loadConfig()
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

	// Setup signal handling for reload and graceful shutdown.
	sigChan := make(chan os.Signal, 1)
	notifySignals(sigChan)

	logger.LogInfo("PacketPony is running", map[string]interface{}{
		"listeners": len(cfg.Listeners),
	})

	for {
		sig := <-sigChan
		if isReloadSignal(sig) {
			logger.LogInfo("Received configuration reload signal", map[string]interface{}{
				"signal": sig.String(),
			})

			nextConfig, loadErr := loadConfig()
			if loadErr != nil {
				logger.LogError("Configuration reload failed", map[string]interface{}{"error": loadErr.Error()})
				continue
			}
			if validateErr := nextConfig.Validate(); validateErr != nil {
				logger.LogError("Reloaded configuration is invalid", map[string]interface{}{"error": validateErr.Error()})
				continue
			}
			if compatibilityErr := config.ValidateReload(cfg, nextConfig); compatibilityErr != nil {
				logger.LogError("Configuration reload requires restart", map[string]interface{}{"error": compatibilityErr.Error()})
				continue
			}
			if reloadErr := manager.Reload(nextConfig); reloadErr != nil {
				logger.LogError("Configuration reload failed", map[string]interface{}{"error": reloadErr.Error()})
				continue
			}

			cfg = nextConfig
			logger.LogInfo("Configuration reloaded successfully", map[string]interface{}{
				"listeners": len(cfg.Listeners),
			})
			continue
		}

		logger.LogInfo("Received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})
		break
	}

	// Graceful shutdown with the most recently loaded timeout.
	if err := manager.GracefulShutdown(cfg.Server.ShutdownTimeout); err != nil {
		logger.LogError("Error during graceful shutdown", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	logger.LogInfo("PacketPony stopped gracefully", nil)
}
