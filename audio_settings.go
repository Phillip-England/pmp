package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
)

type AudioSettings struct {
	WakeWord  string `json:"wake_word"`
	SplitWord string `json:"split_word"`
	SaveWord  string `json:"save_word"`
}

func defaultAudioSettings() AudioSettings {
	return AudioSettings{
		WakeWord:  "giraffe",
		SplitWord: "dash",
		SaveWord:  "cucumber",
	}
}

func normalizeAudioSettings(settings AudioSettings) AudioSettings {
	defaults := defaultAudioSettings()
	settings.WakeWord = strings.TrimSpace(settings.WakeWord)
	settings.SplitWord = strings.TrimSpace(settings.SplitWord)
	settings.SaveWord = strings.TrimSpace(settings.SaveWord)
	if settings.WakeWord == "" {
		settings.WakeWord = defaults.WakeWord
	}
	if settings.SplitWord == "" {
		settings.SplitWord = defaults.SplitWord
	}
	if settings.SaveWord == "" {
		settings.SaveWord = defaults.SaveWord
	}
	return settings
}

func writeDefaultAudioSettings(path string) error {
	settings := defaultAudioSettings()
	bytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func loadAudioSettings() (AudioSettings, error) {
	if err := ensureProject(); err != nil {
		return AudioSettings{}, err
	}

	path, err := audioSettingsPath()
	if err != nil {
		return AudioSettings{}, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := writeDefaultAudioSettings(path); err != nil {
			return AudioSettings{}, err
		}
	} else if err != nil {
		return AudioSettings{}, err
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		return AudioSettings{}, err
	}
	if len(bytes) == 0 {
		settings := defaultAudioSettings()
		if err := saveAudioSettings(settings); err != nil {
			return AudioSettings{}, err
		}
		return settings, nil
	}

	var settings AudioSettings
	if err := json.Unmarshal(bytes, &settings); err != nil {
		return AudioSettings{}, err
	}
	return normalizeAudioSettings(settings), nil
}

func saveAudioSettings(settings AudioSettings) error {
	if err := ensureProject(); err != nil {
		return err
	}
	settings = normalizeAudioSettings(settings)
	path, err := audioSettingsPath()
	if err != nil {
		return err
	}
	bytes, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}
