package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Accounts          []AccountConfig `yaml:"accounts" mapstructure:"accounts"`
	Proxy             string          `yaml:"proxy" mapstructure:"proxy"`                             // socks5://127.0.0.1:1080
	AppID             int             `yaml:"app_id" mapstructure:"app_id"`                           // Optional, account-level config takes priority
	AppHash           string          `yaml:"app_hash" mapstructure:"app_hash"`                       // Optional, account-level config takes priority
	ReplyWaitSeconds  int             `yaml:"reply_wait_seconds" mapstructure:"reply_wait_seconds"`   // Seconds to wait for bot reply, default: 3 seconds
	ReplyHistoryLimit int             `yaml:"reply_history_limit" mapstructure:"reply_history_limit"` // Number of historical messages to fetch, default: 10
	Log               LogConfig       `yaml:"log" mapstructure:"log"`                                 // Logging configuration
	Language          string          `yaml:"language" mapstructure:"language"`                       // Language setting: en | zh, default: en
}

type LogConfig struct {
	Dir    string `yaml:"dir" mapstructure:"dir"`       // Log directory, default: ./log
	Level  string `yaml:"level" mapstructure:"level"`   // Log level, default: info
	Format string `yaml:"format" mapstructure:"format"` // Log format: text (console) or json, default: text
}

type AccountConfig struct {
	Name              string       `yaml:"name" mapstructure:"name"`
	Phone             string       `yaml:"phone" mapstructure:"phone"`
	Password          string       `yaml:"password" mapstructure:"password"` // Two-factor authentication password
	AppID             int          `yaml:"app_id" mapstructure:"app_id"`
	AppHash           string       `yaml:"app_hash" mapstructure:"app_hash"`
	WorkerCount       int          `yaml:"worker_count" mapstructure:"worker_count"`               // Number of concurrent workers, default: 4
	TaskQueueSize     int          `yaml:"task_queue_size" mapstructure:"task_queue_size"`         // Task queue size, default: 100
	ReplyWaitSeconds  int          `yaml:"reply_wait_seconds" mapstructure:"reply_wait_seconds"`   // Seconds to wait for bot reply
	ReplyHistoryLimit int          `yaml:"reply_history_limit" mapstructure:"reply_history_limit"` // Number of historical messages to fetch
	Tasks             []TaskConfig `yaml:"tasks" mapstructure:"tasks"`
}

type TaskConfig struct {
	Name              string `yaml:"name" mapstructure:"name"`                               // Task name for identification
	Target            string `yaml:"target" mapstructure:"target"`                           // Target username or ID
	Method            string `yaml:"method" mapstructure:"method"`                           // message or button
	Payload           string `yaml:"payload" mapstructure:"payload"`                         // Message content or button text
	Schedule          string `yaml:"schedule" mapstructure:"schedule"`                       // Cron expression or @every 1h
	Enabled           *bool  `yaml:"enabled" mapstructure:"enabled"`                         // Enabled by default
	RunOnStart        bool   `yaml:"run_on_start" mapstructure:"run_on_start"`               // Execute once on startup when true
	ReplyWaitSeconds  int    `yaml:"reply_wait_seconds" mapstructure:"reply_wait_seconds" `  // Seconds to wait for bot reply
	ReplyHistoryLimit int    `yaml:"reply_history_limit" mapstructure:"reply_history_limit"` // Number of historical messages to fetch
}

func LoadConfig(path string, v *viper.Viper) (*Config, error) {
	v.SetConfigFile(path)

	// Support environment variable override
	// Environment variable naming rule: TG_ + config path (separated by underscore)
	// Example: TG_LOG_LEVEL, TG_ACCOUNTS_0_PHONE, TG_APP_ID
	v.SetEnvPrefix("TG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read main config file
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	// Try to merge environment-specific config file (e.g. config.test.yaml, config.prod.yaml)
	// Priority: environment config > main config
	if env := os.Getenv("APP_ENV"); env != "" {
		// Build environment config file name
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		name := strings.TrimSuffix(base, ext)
		envConfigPath := filepath.Join(dir, fmt.Sprintf("%s.%s%s", name, env, ext))

		// Merge if environment config file exists
		if _, err := os.Stat(envConfigPath); err == nil {
			v.SetConfigFile(envConfigPath)
			if err := v.MergeInConfig(); err != nil {
				return nil, fmt.Errorf("failed to merge config file %s: %w", envConfigPath, err)
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func MergeConfig(base, override *Config) (*Config, error) {
	if base == nil {
		return override, nil
	}
	if override == nil {
		return base, nil
	}

	merged := *base
	if override.Proxy != "" {
		merged.Proxy = override.Proxy
	}
	if override.AppID != 0 {
		merged.AppID = override.AppID
	}
	if override.AppHash != "" {
		merged.AppHash = override.AppHash
	}

	if len(override.Accounts) > 0 {
		accounts, err := mergeAccounts(base.Accounts, override.Accounts)
		if err != nil {
			return nil, err
		}
		merged.Accounts = accounts
	}

	return &merged, nil
}

func mergeAccounts(base, override []AccountConfig) ([]AccountConfig, error) {
	if len(override) == 0 {
		return base, nil
	}
	if allAccountsNamed(base) && allAccountsNamed(override) {
		return mergeAccountsByName(base, override), nil
	}
	if len(base) != len(override) {
		return nil, fmt.Errorf("accounts length mismatch: base=%d override=%d", len(base), len(override))
	}
	return mergeAccountsByIndex(base, override), nil
}

func mergeAccount(base, override AccountConfig) AccountConfig {
	merged := base
	if override.Name != "" {
		merged.Name = override.Name
	}
	if override.Phone != "" {
		merged.Phone = override.Phone
	}
	if override.Password != "" {
		merged.Password = override.Password
	}
	if override.AppID != 0 {
		merged.AppID = override.AppID
	}
	if override.AppHash != "" {
		merged.AppHash = override.AppHash
	}
	if len(override.Tasks) > 0 {
		merged.Tasks = mergeTasks(base.Tasks, override.Tasks)
	}
	return merged
}

func allAccountsNamed(accounts []AccountConfig) bool {
	if len(accounts) == 0 {
		return false
	}
	for _, acc := range accounts {
		if strings.TrimSpace(acc.Name) == "" {
			return false
		}
	}
	return true
}

func mergeAccountsByName(base, override []AccountConfig) []AccountConfig {
	overrideIndex := make(map[string]AccountConfig, len(override))
	for _, acc := range override {
		overrideIndex[strings.TrimSpace(acc.Name)] = acc
	}

	merged := make([]AccountConfig, 0, len(base))
	seen := make(map[string]struct{}, len(override))
	for _, acc := range base {
		key := strings.TrimSpace(acc.Name)
		if over, ok := overrideIndex[key]; ok {
			merged = append(merged, mergeAccount(acc, over))
			seen[key] = struct{}{}
		} else {
			merged = append(merged, acc)
		}
	}

	for _, acc := range override {
		key := strings.TrimSpace(acc.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, acc)
	}

	return merged
}

func mergeAccountsByIndex(base, override []AccountConfig) []AccountConfig {
	maxLen := len(base)
	if len(override) > maxLen {
		maxLen = len(override)
	}

	merged := make([]AccountConfig, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		var b AccountConfig
		var o AccountConfig
		hasBase := i < len(base)
		hasOverride := i < len(override)
		if hasBase {
			b = base[i]
		}
		if hasOverride {
			o = override[i]
		}

		switch {
		case hasBase && hasOverride:
			merged = append(merged, mergeAccount(b, o))
		case hasBase:
			merged = append(merged, b)
		default:
			merged = append(merged, o)
		}
	}

	return merged
}

func mergeTasks(base, override []TaskConfig) []TaskConfig {
	maxLen := len(base)
	if len(override) > maxLen {
		maxLen = len(override)
	}

	merged := make([]TaskConfig, 0, maxLen)
	for i := 0; i < maxLen; i++ {
		var b TaskConfig
		var o TaskConfig
		hasBase := i < len(base)
		hasOverride := i < len(override)
		if hasBase {
			b = base[i]
		}
		if hasOverride {
			o = override[i]
		}

		switch {
		case hasBase && hasOverride:
			merged = append(merged, mergeTask(b, o))
		case hasBase:
			merged = append(merged, b)
		default:
			merged = append(merged, o)
		}
	}

	return merged
}

func mergeTask(base, override TaskConfig) TaskConfig {
	merged := base
	if override.Target != "" {
		merged.Target = override.Target
	}
	if override.Method != "" {
		merged.Method = override.Method
	}
	if override.Payload != "" {
		merged.Payload = override.Payload
	}
	if override.Schedule != "" {
		merged.Schedule = override.Schedule
	}
	if override.Enabled != nil {
		merged.Enabled = override.Enabled
	}
	if override.RunOnStart {
		merged.RunOnStart = true
	}
	return merged
}
