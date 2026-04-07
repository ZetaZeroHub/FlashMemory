package common

import (
	"os"
	"strings"
)

var globalLang string

// SetLang explicitly sets the language
func SetLang(lang string) {
	globalLang = lang
}

// GetLang returns the current language preference. Supported: "en", "zh"
func GetLang() string {
	if globalLang != "" {
		return globalLang
	}

	// 1. Env FM_LANG
	if l := os.Getenv("FM_LANG"); l != "" {
		if strings.HasPrefix(strings.ToLower(l), "zh") {
			return "zh"
		}
		return "en"
	}

	// 2. System LANG
	if l := os.Getenv("LANG"); l != "" {
		if strings.HasPrefix(strings.ToLower(l), "zh") {
			return "zh"
		}
	}
	
	// Default English
	return "en"
}

// IsZH is a helper to check if the language is Chinese
func IsZH() bool {
	return GetLang() == "zh"
}
