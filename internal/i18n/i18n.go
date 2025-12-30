package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"
)

var bundle *i18n.Bundle
var localizer *i18n.Localizer

// Init Initialize internationalization support
func Init(lang string) error {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exePath)

	// Try multiple possible locales directory locations
	localeDirs := []string{
		filepath.Join(exeDir, "locales"),       // Same directory as executable
		filepath.Join(exeDir, "..", "locales"), // Development environment
		"locales",                              // Current directory
	}

	var localeDir string
	for _, dir := range localeDirs {
		if _, err := os.Stat(dir); err == nil {
			localeDir = dir
			break
		}
	}

	if localeDir == "" {
		localeDir = "locales" // Fallback to default
	}

	// Load English (default)
	enFile := filepath.Join(localeDir, "en.yaml")
	if _, err := bundle.LoadMessageFile(enFile); err != nil {
		return err
	}

	// Load Chinese
	zhFile := filepath.Join(localeDir, "zh.yaml")
	if _, err := bundle.LoadMessageFile(zhFile); err != nil {
		return err
	}

	// Set language based on configuration
	if lang == "" {
		lang = "en"
	}
	localizer = i18n.NewLocalizer(bundle, lang)

	return nil
}

// T Translation function
func T(messageID string) string {
	if localizer == nil {
		return messageID
	}

	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID: messageID,
	})
	if err != nil {
		return messageID
	}
	return msg
}

// SetLanguage Dynamically switch language
func SetLanguage(lang string) {
	if bundle == nil {
		return
	}
	localizer = i18n.NewLocalizer(bundle, lang)
}
