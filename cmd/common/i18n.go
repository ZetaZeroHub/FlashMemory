package common

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var globalLang string

// SetLang explicitly sets the language and persists it to config file
func SetLang(lang string) {
	globalLang = lang
	// Persist to config file
	persistLangToConfig(lang)
}

// GetLang returns the current language preference. Supported: "en", "zh"
// Priority: explicit SetLang > config file > env FM_LANG > "en" (default)
func GetLang() string {
	if globalLang != "" {
		return globalLang
	}

	// 1. Read from config file (~/.flashmemory/config.yaml)
	if l := readLangFromConfig(); l != "" {
		globalLang = l // cache it
		return l
	}

	// 2. Env FM_LANG
	if l := os.Getenv("FM_LANG"); l != "" {
		if strings.HasPrefix(strings.ToLower(l), "zh") {
			return "zh"
		}
		return "en"
	}

	// 3. Default English (NOT system locale — explicit design decision)
	return "en"
}

// IsZH is a helper to check if the language is Chinese
func IsZH() bool {
	return GetLang() == "zh"
}

// I18n returns the Chinese or English string based on current language
func I18n(zh, en string) string {
	if IsZH() {
		return zh
	}
	return en
}

// readLangFromConfig reads the "lang" field from ~/.flashmemory/config.yaml
func readLangFromConfig() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	configPath := filepath.Join(home, ".flashmemory", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ""
	}

	if l, ok := cfg["lang"].(string); ok && l != "" {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "zh" || l == "en" {
			return l
		}
	}
	return ""
}

// persistLangToConfig writes the lang setting into ~/.flashmemory/config.yaml
func persistLangToConfig(lang string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	configPath := filepath.Join(home, ".flashmemory", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return // config file doesn't exist yet, skip
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	cfg["lang"] = lang

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return
	}

	header := "# FlashMemory Configuration\n\n"
	os.WriteFile(configPath, append([]byte(header), out...), 0644)
}
