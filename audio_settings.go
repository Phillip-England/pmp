package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const systemSettingsFileName = "settings.json"

type ThemeSettings struct {
	AccentColor string `json:"accent_color"`
}

type ProjectSettings struct {
	ScanRoots []string `json:"scan_roots"`
}

type SystemSettings struct {
	Theme    ThemeSettings   `json:"theme"`
	Projects ProjectSettings `json:"projects"`
}

var hexColorPattern = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func defaultThemeSettings() ThemeSettings {
	return ThemeSettings{AccentColor: "#8fd18a"}
}

func defaultSystemSettings() SystemSettings {
	return SystemSettings{
		Theme:    defaultThemeSettings(),
		Projects: defaultProjectSettings(),
	}
}

func defaultProjectSettings() ProjectSettings {
	return ProjectSettings{ScanRoots: defaultProjectScanRoots()}
}

func normalizeThemeSettings(settings ThemeSettings) ThemeSettings {
	defaults := defaultThemeSettings()
	settings.AccentColor = strings.TrimSpace(strings.ToLower(settings.AccentColor))
	if hexColorPattern.MatchString(settings.AccentColor) {
		return settings
	}
	return defaults
}

func defaultProjectScanRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}
	return normalizeProjectScanRoots([]string{
		filepath.Join(home, "Desktop", "src"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Documents", "Projects"),
	})
}

func normalizeProjectScanRoots(roots []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		absRoot = filepath.Clean(absRoot)
		if shouldIgnoreProjectPath(absRoot) || seen[absRoot] {
			continue
		}
		seen[absRoot] = true
		normalized = append(normalized, absRoot)
	}
	return normalized
}

func normalizeProjectSettings(settings ProjectSettings) ProjectSettings {
	settings.ScanRoots = normalizeProjectScanRoots(settings.ScanRoots)
	if len(settings.ScanRoots) == 0 {
		settings.ScanRoots = defaultProjectScanRoots()
	}
	return settings
}

func normalizeSystemSettings(settings SystemSettings) SystemSettings {
	settings.Theme = normalizeThemeSettings(settings.Theme)
	settings.Projects = normalizeProjectSettings(settings.Projects)
	return settings
}

func configRootDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PMP_CONFIG_HOME")); override != "" {
		return override, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "pmp"), nil
}

func systemSettingsPath() (string, error) {
	root, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, systemSettingsFileName), nil
}

func writeDefaultSystemSettings(path string) error {
	settings := defaultSystemSettings()
	bytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
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
