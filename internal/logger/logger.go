package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// SetupLogger sets up basic console logger
func SetupLogger(levelStr string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006/01/02 15:04:05",
	}
	logger := zerolog.New(output).With().Timestamp().Logger()

	level := zerolog.InfoLevel
	if strings.TrimSpace(levelStr) != "" {
		parsed, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(levelStr)))
		if err == nil {
			level = parsed
		} else {
			fmt.Fprintf(os.Stderr, "invalid --log-level=%q, fallback to %s\n", levelStr, level.String())
		}
	}
	zerolog.SetGlobalLevel(level)

	if level == zerolog.DebugLevel {
		logger.Debug().Msg("Debug mode enabled")
	}

	return logger
}

// SetupLoggerWithFile sets up logger with console and file output
func SetupLoggerWithFile(levelStr string, logDir string, format string) (zerolog.Logger, error) {
	// Set default log directory
	if logDir == "" {
		logDir = "./log"
	}

	// Set default format
	if format == "" {
		format = "text"
	}

	// Create log directory
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return zerolog.Logger{}, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Set time format
	zerolog.TimeFieldFormat = time.RFC3339

	// File output - app.log (append mode)
	appLogPath := filepath.Join(logDir, "app.log")
	appLogFile, err := os.OpenFile(appLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return zerolog.Logger{}, fmt.Errorf("failed to open app.log: %w", err)
	}

	// Console output (based on format)
	var consoleWriter io.Writer
	var fileWriter io.Writer
	if format == "json" {
		consoleWriter = os.Stdout
		fileWriter = appLogFile
	} else {
		consoleWriter = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006/01/02 15:04:05",
		}
		fileWriter = zerolog.ConsoleWriter{
			Out:        appLogFile,
			TimeFormat: "2006/01/02 15:04:05",
			NoColor:    true, // No color in file
		}
	}

	// Multiple outputs: console + file
	multiWriter := io.MultiWriter(consoleWriter, fileWriter)
	logger := zerolog.New(multiWriter).With().Timestamp().Logger()

	// Set log level
	level := zerolog.InfoLevel
	if strings.TrimSpace(levelStr) != "" {
		parsed, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(levelStr)))
		if err == nil {
			level = parsed
		} else {
			logger.Warn().Str("invalid_level", levelStr).Str("fallback", level.String()).Msg("Invalid log level")
		}
	}
	zerolog.SetGlobalLevel(level)

	if level == zerolog.DebugLevel {
		logger.Debug().Msg("Debug mode enabled")
	}

	logger.Info().
		Str("log_dir", logDir).
		Str("app_log", appLogPath).
		Str("format", format).
		Str("level", level.String()).
		Msg("Logging system initialized")

	return logger, nil
}

// CreateTaskLogger creates separate log file for task
func CreateTaskLogger(logDir string, accountName string, taskName string, triggerType string, format string) (zerolog.Logger, *os.File, error) {
	if logDir == "" {
		logDir = "./log"
	}

	// Create task log subdirectory
	taskLogDir := filepath.Join(logDir, "tasks")
	if err := os.MkdirAll(taskLogDir, 0755); err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("failed to create task log directory: %w", err)
	}

	// File format: account_task_triggerType_timestamp.log
	timestamp := time.Now().Format("20060102_150405")
	safeAccountName := sanitizeFilename(accountName)
	safeTaskName := sanitizeFilename(taskName)

	filename := fmt.Sprintf("%s_%s_%s_%s.log", safeAccountName, safeTaskName, triggerType, timestamp)
	logPath := filepath.Join(taskLogDir, filename)

	// Create task log file (new file mode)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("failed to create task log file: %w", err)
	}

	// Select log format based on format config
	var logger zerolog.Logger
	if format == "json" {
		// JSON format
		logger = zerolog.New(logFile).With().
			Timestamp().
			Str("account", accountName).
			Str("task", taskName).
			Str("trigger", triggerType).
			Logger()
	} else {
		// Text format (console format)
		consoleWriter := zerolog.ConsoleWriter{
			Out:        logFile,
			TimeFormat: "2006/01/02 15:04:05",
			NoColor:    true, // No color in file
		}
		logger = zerolog.New(consoleWriter).With().
			Timestamp().
			Str("account", accountName).
			Str("task", taskName).
			Str("trigger", triggerType).
			Logger()
	}

	return logger, logFile, nil
}

// sanitizeFilename removes illegal characters from filename
func sanitizeFilename(name string) string {
	// Remove or replace illegal characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
		"@", "",
		"+", "",
	)
	return replacer.Replace(name)
}
