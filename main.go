package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"telegram-auto-checkin/internal/config"
	"telegram-auto-checkin/internal/i18n"
	"telegram-auto-checkin/internal/logger"
	"telegram-auto-checkin/internal/scheduler"
)

var (
	runOnce    = flag.Bool("once", false, "Run all tasks once and exit")
	logLevel   = flag.String("log-level", "", "Log level: debug|info|warn|error (default: info)")
	configPath = flag.String("config", "config.yaml", "Path to main config file (YAML)")

	log zerolog.Logger
)

func main() {
	flag.Parse()

	// Initialize viper
	v := viper.New()

	// Bind command line flags to viper (can override config file)
	if *logLevel != "" {
		v.Set("log.level", *logLevel)
	}

	// Use default console logger first, initialize file logger after loading config
	log = logger.SetupLogger(*logLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadConfig(*configPath, v)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load configuration")
		os.Exit(1)
	}

	// Initialize internationalization
	lang := cfg.Language
	if lang == "" {
		lang = "en"
	}
	if err := i18n.Init(lang); err != nil {
		log.Warn().Err(err).Str("language", lang).Msg("Failed to initialize i18n, using default")
	} else {
		log.Info().Str("language", lang).Msg("Language initialized")
	}

	// Reinitialize logging system with config directory
	// Command line flags have higher priority than config file
	effectiveLogLevel := cfg.Log.Level
	if *logLevel != "" {
		effectiveLogLevel = *logLevel
	}
	fileLogger, err := logger.SetupLoggerWithFile(effectiveLogLevel, cfg.Log.Dir, cfg.Log.Format)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize file logging system")
		os.Exit(1)
	}
	log = fileLogger

	// Print configuration info for verification
	appEnv := os.Getenv("APP_ENV")
	if appEnv != "" {
		log.Info().Str("environment", appEnv).Msg("Using environment-specific config")
	}

	log.Info().
		Int("accounts", len(cfg.Accounts)).
		Bool("once_mode", *runOnce).
		Str("config", *configPath).
		Str("log_format", cfg.Log.Format).
		Str("log_level", cfg.Log.Level).
		Str("proxy", cfg.Proxy).
		Msg("Configuration loaded successfully")

	if *runOnce {
		if err := scheduler.RunTasksOnce(ctx, cfg, log); err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info().Msg("Tasks cancelled")
				os.Exit(0)
			}
			log.Error().Err(err).Msg("Task execution failed")
			os.Exit(1)
		}
		log.Info().Msg("All tasks completed, exiting")
		return
	}

	if err := scheduler.RunTasks(ctx, cfg, log); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info().Msg("Scheduled tasks cancelled")
			os.Exit(0)
		}
		log.Error().Err(err).Msg("Failed to initialize scheduled tasks")
		os.Exit(1)
	}

	<-ctx.Done()
	log.Info().Msg("Received exit signal, shutting down...")
}
