package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/consumer"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/health"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/rule"
)

var Version = "dev"

func flagOrEnv(flagValue string, envName string, defaultValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return defaultValue
}

func main() {
	if Version == "dev" || Version == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				Version = bi.Main.Version
			}
		}
	}

	// Parse command line flags
	configFlag := flag.String("config", "", "path to YAML config file")
	portFlag := flag.String("port", "", "health endpoint port")
	flag.Parse()

	// Set up structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Log the version
	logger.Info("starting ghwhenthen", "version", Version)

	// Resolve flags with environment variables
	configPath := flagOrEnv(*configFlag, "GHWHENTHEN_CONFIG", "")
	port := flagOrEnv(*portFlag, "GHWHENTHEN_PORT", "8080")

	if configPath == "" {
		logger.Error("config file path is required (--config or GHWHENTHEN_CONFIG)")
		os.Exit(1)
	}

	logger.Info("configuration", "config", configPath, "port", port)

	// Load and validate config
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := config.Validate(cfg); err != nil {
		logger.Error("failed to validate config", "error", err)
		os.Exit(1)
	}

	// Count enabled rules
	enabledCount := 0
	for _, r := range cfg.Rules {
		if r.Enabled {
			enabledCount++
		}
	}
	logger.Info("rules loaded", "total", len(cfg.Rules), "enabled", enabledCount)

	// Read the GitHub token from the environment variable specified in config
	token := os.Getenv(cfg.GitHub.TokenEnvVar)
	if token == "" {
		logger.Error("GitHub token environment variable is empty", "env_var", cfg.GitHub.TokenEnvVar)
		os.Exit(1)
	}

	// Create the rule engine
	engine, err := rule.NewEngine(cfg.Rules, cfg.Constants, cfg.GitHub.GraphQLEndpoint, token)
	if err != nil {
		logger.Error("failed to create rule engine", "error", err)
		os.Exit(1)
	}

	// Create health status and set alive
	healthStatus := health.NewStatus()
	healthStatus.SetAlive(true)

	// Start health HTTP server in a goroutine
	healthMux := http.NewServeMux()
	healthMux.Handle("/", healthStatus.Handler())
	healthServer := &http.Server{
		Handler: healthMux,
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Error("failed to listen on health port", "port", port, "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("health server starting", "port", port)
		if err := healthServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error("health server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Create the consumer
	c := consumer.New(
		cfg.PubSub.ProjectID,
		cfg.PubSub.SubscriptionID,
		cfg.Behavior.OnFailure,
		engine,
		healthStatus,
		consumer.WithLogger(logger),
	)

	// Handle OS signals for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Run the consumer (blocks until context cancelled)
	if err := c.Run(ctx); err != nil {
		logger.Error("consumer exited with error", "error", err)
	}

	// Graceful shutdown of health server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("health server shutdown error", "error", err)
	}

	logger.Info(fmt.Sprintf("ghwhenthen %s exiting", Version))
}
