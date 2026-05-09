package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const systemSettingsFileName = "settings.json"
const legacyAudioSettingsFileName = "audio-settings.json"

type AudioSettings struct {
	SplitWord string `json:"split_word"`
	SaveWord  string `json:"save_word"`
}

type ThemeSettings struct {
	AccentColor string `json:"accent_color"`
}

type SystemSettings struct {
	Audio AudioSettings `json:"audio"`
	Theme ThemeSettings `json:"theme"`
}

var hexColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func defaultAudioSettings() AudioSettings {
	return AudioSettings{
		SplitWord: "dash",
		SaveWord:  "cucumber",
	}
}

func defaultThemeSettings() ThemeSettings {
	return ThemeSettings{AccentColor: "#8fd18a"}
}

func defaultSystemSettings() SystemSettings {
	return SystemSettings{
		Audio: defaultAudioSettings(),
		Theme: defaultThemeSettings(),
	}
}

func normalizeAudioSettings(settings AudioSettings) AudioSettings {
	defaults := defaultAudioSettings()
	settings.SplitWord = strings.TrimSpace(settings.SplitWord)
	settings.SaveWord = strings.TrimSpace(settings.SaveWord)
	if settings.SplitWord == "" {
		settings.SplitWord = defaults.SplitWord
	}
	if settings.SaveWord == "" {
		settings.SaveWord = defaults.SaveWord
	}
	return settings
}

func normalizeThemeSettings(settings ThemeSettings) ThemeSettings {
	defaults := defaultThemeSettings()
	settings.AccentColor = strings.TrimSpace(strings.ToLower(settings.AccentColor))
	if hexColorPattern.MatchString(settings.AccentColor) {
		return settings
	}
	return defaults
}

func normalizeSystemSettings(settings SystemSettings) SystemSettings {
	settings.Audio = normalizeAudioSettings(settings.Audio)
	settings.Theme = normalizeThemeSettings(settings.Theme)
	return settings
}

func systemSettingsPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PMP_CONFIG_HOME")); override != "" {
		return filepath.Join(override, systemSettingsFileName), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "pmp", systemSettingsFileName), nil
}

func writeDefaultSystemSettings(path string) error {
	settings, err := initialSystemSettings()
	if err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func initialSystemSettings() (SystemSettings, error) {
	settings := defaultSystemSettings()
	legacyPath, err := legacyAudioSettingsPath()
	if err != nil {
		return settings, nil
	}
	bytes, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return SystemSettings{}, err
	}
	var audioSettings AudioSettings
	if err := json.Unmarshal(bytes, &audioSettings); err != nil {
		return settings, nil
	}
	settings.Audio = normalizeAudioSettings(audioSettings)
	return settings, nil
}

func legacyAudioSettingsPath() (string, error) {
	root, err := projectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, projectDirName, legacyAudioSettingsFileName), nil
}

func loadSystemSettings() (SystemSettings, error) {
	path, err := systemSettingsPath()
	if err != nil {
		return SystemSettings{}, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := writeDefaultSystemSettings(path); err != nil {
			return SystemSettings{}, err
		}
	} else if err != nil {
		return SystemSettings{}, err
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return SystemSettings{}, err
	}
	if len(bytes) == 0 {
		settings := defaultSystemSettings()
		if err := saveSystemSettings(settings); err != nil {
			return SystemSettings{}, err
		}
		return settings, nil
	}

	var settings SystemSettings
	if err := json.Unmarshal(bytes, &settings); err != nil {
		return SystemSettings{}, err
	}
	return normalizeSystemSettings(settings), nil
}

func saveSystemSettings(settings SystemSettings) error {
	path, err := systemSettingsPath()
	if err != nil {
		return err
	}
	settings = normalizeSystemSettings(settings)
	bytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func loadAudioSettings() (AudioSettings, error) {
	settings, err := loadSystemSettings()
	if err != nil {
		return AudioSettings{}, err
	}
	return settings.Audio, nil
}

func saveAudioSettings(settings AudioSettings) error {
	systemSettings, err := loadSystemSettings()
	if err != nil {
		return err
	}
	systemSettings.Audio = normalizeAudioSettings(settings)
	return saveSystemSettings(systemSettings)
}

func loadThemeSettings() (ThemeSettings, error) {
	settings, err := loadSystemSettings()
	if err != nil {
		return ThemeSettings{}, err
	}
	return settings.Theme, nil
}

func saveThemeSettings(settings ThemeSettings) error {
	systemSettings, err := loadSystemSettings()
	if err != nil {
		return err
	}
	systemSettings.Theme = normalizeThemeSettings(settings)
	return saveSystemSettings(systemSettings)
}
