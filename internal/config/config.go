package config

import (
	"clearlink/internal/models"
	"clearlink/internal/network"
	"fmt"
	"os"
	"strings"

	"gopkg.in/ini.v1"
)

var (
	Config   *models.Config
	FilePath string
)

var ConfigEntries = []models.ConfigEntry{
	// SERVER
	{Key: "Port", Type: models.ApplicationTypeServer, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4125}},
	{Key: "WebPort", Type: models.ApplicationTypeServer, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 8080}},
	{Key: "AuthKey", Type: models.ApplicationTypeServer, Default: models.EntryVar{Type: models.EntryTypeString, Data: "Default"}},
	{Key: "AdminUsername", Type: models.ApplicationTypeServer, Default: models.EntryVar{Type: models.EntryTypeString, Data: "admin"}},
	{Key: "AdminPassword", Type: models.ApplicationTypeServer, Default: models.EntryVar{Type: models.EntryTypeString, Data: "change-me"}},
	// BROADCAST
	{Key: "AuthKey", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: "Default"}},
	{Key: "ServerPort", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4125}},
	{Key: "ServerAddr", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: "127.0.0.1"}},
	{Key: "NodeName", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: "DefaultBroadcastNodeName"}},
	{Key: "Type", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: "DISCORD"}},
	{Key: "Ptt_Pin", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4}},
	{Key: "BotToken", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: ""}},
	{Key: "GuildID", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: ""}},
	{Key: "VoiceChannelID", Type: models.ApplicationTypeBroadcast, Default: models.EntryVar{Type: models.EntryTypeString, Data: ""}},
	// INTERFACE
	{Key: "AuthKey", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeString, Data: "Default"}},
	{Key: "ServerPort", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4125}},
	{Key: "ServerAddr", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeString, Data: "127.0.0.1"}},
	{Key: "NodeName", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeString, Data: "DefaultInterfaceNodeName"}},
	{Key: "InputDevice", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeString, Data: "default"}},
	{Key: "OutputDevice", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeString, Data: "default"}},
	{Key: "AudioRate", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 48000}},
	{Key: "AudioChunkMs", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 20}},
	{Key: "Squelch_Pin", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 17}},
	{Key: "Squelch_Active_High", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
	{Key: "Ptt_Pin", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4}},
	{Key: "Ptt_Active_High", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
	{Key: "RouteToBroadcast", Type: models.ApplicationTypeInterface, Default: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
	// LISTEN
	{Key: "AuthKey", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeString, Data: "Default"}},
	{Key: "ServerPort", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4125}},
	{Key: "ServerAddr", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeString, Data: "127.0.0.1"}},
	{Key: "NodeName", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeString, Data: "DefaultNodeName"}},
	{Key: "Frequency", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 146520000}},
	{Key: "CenterFrequency", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 0}},
	{Key: "SampleRate", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 960000}},
	{Key: "AudioRate", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 48000}},
	{Key: "AudioChunkMs", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 20}},
	{Key: "BufferSize", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 16384}},
	{Key: "AudioGain", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 28000}},
	{Key: "FMDeviation", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 3000}},
	{Key: "DeviceIndex", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 0}},
	{Key: "DirectSampling", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 0}},
	{Key: "TunerBandwidth", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 15000}},
	{Key: "TunerGain", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 254}},
	{Key: "Bandwidth", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 0}},
	{Key: "SquelchThreshold", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 0}},
	{Key: "SquelchSNRThreshold", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeFloat, Data: 9.54}},
	{Key: "CTCSS", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeFloat, Data: 0.0}},
	{Key: "Notch", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeFloat, Data: 0.0}},
	{Key: "NotchQ", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeFloat, Data: 10.0}},
	{Key: "AmpFactor", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeFloat, Data: 1.0}},
	{Key: "Highpass", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 100}},
	{Key: "Lowpass", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 2500}},
	{Key: "Tau", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 200}},
	{Key: "RouteToBroadcast", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
	{Key: "RouteToInterface", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
}

func configEntryDefaultValue(entry models.ConfigEntry, section *ini.Section) string {
	if entry.Type == models.ApplicationTypeListen {
		legacyKey := map[string]string{
			"Lowpass":          "AudioCutoffHz",
			"SquelchThreshold": "SquelchDB",
			"Tau":              "DeemphasisTauUs",
		}[entry.Key]
		if legacyKey != "" && section.HasKey(legacyKey) {
			return section.Key(legacyKey).String()
		}
	}

	switch entry.Default.Type {
	case models.EntryTypeInt:
		return fmt.Sprintf("%d", entry.Default.Data.(int))
	case models.EntryTypeString:
		return entry.Default.Data.(string)
	case models.EntryTypeFloat:
		return fmt.Sprintf("%g", entry.Default.Data.(float64))
	case models.EntryTypeBool:
		return fmt.Sprintf("%t", entry.Default.Data.(bool))
	default:
		return ""
	}
}

// LoadConfig loads Config from an INI file
func LoadConfig(path string, app_type models.ApplicationType) (*models.Config, error) {
	err := InitConfig(path, app_type)
	if err != nil {
		return nil, err
	}
	cfgFile, err := ini.Load(path)
	if err != nil {
		return nil, err
	}
	cfg := &models.Config{}

	for _, entry := range ConfigEntries {
		if entry.Type != app_type {
			continue
		}
		var value any

		switch entry.Default.Type {
		case models.EntryTypeInt:
			value = cfgFile.Section(string(app_type)).Key(entry.Key).MustInt(entry.Default.Data.(int))
		case models.EntryTypeString:
			value = cfgFile.Section(string(app_type)).Key(entry.Key).MustString(entry.Default.Data.(string))
		case models.EntryTypeFloat:
			value = cfgFile.Section(string(app_type)).Key(entry.Key).MustFloat64(entry.Default.Data.(float64))
		case models.EntryTypeBool:
			value = cfgFile.Section(string(app_type)).Key(entry.Key).MustBool(entry.Default.Data.(bool))
		}

		cfg.Entries = append(cfg.Entries, models.ConfigEntry{
			Key:     entry.Key,
			Type:    entry.Type,
			Var:     models.EntryVar{Type: entry.Default.Type, Data: value},
			Default: entry.Default,
		})
	}
	Config = cfg
	return cfg, nil
}

// InitConfig verifies the config file exists, and creates it if it doesn't, then sets the default values
func InitConfig(path string, app_type models.ApplicationType) error {
	FilePath = path
	// check if config exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		cfg := ini.Empty()

		for _, entry := range ConfigEntries {
			if entry.Type != app_type {
				continue
			}
			cfg.Section(string(app_type)).Key(entry.Key).SetValue(configEntryDefaultValue(entry, cfg.Section(string(app_type))))
		}

		cfg.SaveTo(FilePath)

		fmt.Println("Config file created. Please review and edit the values as needed before restarting the application.")
		os.Exit(0)
	} else if err != nil {
		return err
	} else {
		cfgFile, err := ini.Load(FilePath)
		if err != nil {
			return err
		}
		has_updated := false
		for _, entry := range ConfigEntries {
			if entry.Type != app_type {
				continue
			}
			if !cfgFile.Section(string(app_type)).HasKey(entry.Key) {
				has_updated = true
				section := cfgFile.Section(string(app_type))
				section.Key(entry.Key).SetValue(configEntryDefaultValue(entry, section))
			}
		}

		cfgFile.SaveTo(path)

		if has_updated {
			fmt.Println("Config file updated. Please review and edit the values as needed before restarting the application.")
			os.Exit(0)
		}
	}
	return nil
}

func GetConfigValue(key string, app_type models.ApplicationType) any {
	for _, entry := range Config.Entries {
		if entry.Key == key && entry.Type == app_type {
			return entry.Var.Data
		}
	}
	return nil
}

func UpdateConfigValue(entry models.ConfigEntry) {
	for i, e := range Config.Entries {
		if e.Key == entry.Key && e.Type == entry.Type {
			Config.Entries[i].Var.Data = entry.Var.Data
			cfg, err := ini.Load(FilePath)
			if err != nil {
				cfg = ini.Empty()
			}

			for _, entry := range Config.Entries {
				switch entry.Default.Type {
				case models.EntryTypeInt:
					cfg.Section(string(entry.Type)).Key(entry.Key).SetValue(fmt.Sprintf("%d", entry.Var.Data.(int)))
				case models.EntryTypeString:
					cfg.Section(string(entry.Type)).Key(entry.Key).SetValue(entry.Var.Data.(string))
				case models.EntryTypeFloat:
					cfg.Section(string(entry.Type)).Key(entry.Key).SetValue(fmt.Sprintf("%g", entry.Var.Data.(float64)))
				case models.EntryTypeBool:
					cfg.Section(string(entry.Type)).Key(entry.Key).SetValue(fmt.Sprintf("%t", entry.Var.Data.(bool)))
				}
			}

			cfg.SaveTo(FilePath)
			return
		}
	}
}

// ValidateInterfaceConfig verifies that an Interface node can use its soundcard and GPIO settings.
func ValidateInterfaceConfig(cfg models.Config) error {
	inputDevice, err := interfaceConfigString(cfg.Entries, "InputDevice")
	if err != nil {
		return err
	}
	if strings.TrimSpace(inputDevice) == "" {
		return fmt.Errorf("InputDevice cannot be empty")
	}

	outputDevice, err := interfaceConfigString(cfg.Entries, "OutputDevice")
	if err != nil {
		return err
	}
	if strings.TrimSpace(outputDevice) == "" {
		return fmt.Errorf("OutputDevice cannot be empty")
	}

	audioRate, err := interfaceConfigInt(cfg.Entries, "AudioRate")
	if err != nil {
		return err
	}
	if audioRate < 8000 || audioRate > 192000 {
		return fmt.Errorf("AudioRate %d must be between 8000 and 192000", audioRate)
	}

	audioChunkMs, err := interfaceConfigInt(cfg.Entries, "AudioChunkMs")
	if err != nil {
		return err
	}
	if audioChunkMs < 5 || audioChunkMs > 1000 {
		return fmt.Errorf("AudioChunkMs %d must be between 5 and 1000", audioChunkMs)
	}
	if audioRate*audioChunkMs%1000 != 0 {
		return fmt.Errorf("AudioRate multiplied by AudioChunkMs must be divisible by 1000")
	}
	if sampleCount := audioRate * audioChunkMs / 1000; sampleCount > network.MaxAudioChunkSamples {
		return fmt.Errorf("audio chunk has %d samples, maximum is %d", sampleCount, network.MaxAudioChunkSamples)
	}

	squelchPin, err := interfaceConfigInt(cfg.Entries, "Squelch_Pin")
	if err != nil {
		return err
	}
	if squelchPin < 0 {
		return fmt.Errorf("Squelch_Pin must be non-negative")
	}
	pttPin, err := interfaceConfigInt(cfg.Entries, "Ptt_Pin")
	if err != nil {
		return err
	}
	if pttPin < 0 {
		return fmt.Errorf("Ptt_Pin must be non-negative")
	}
	if squelchPin == pttPin {
		return fmt.Errorf("Squelch_Pin and Ptt_Pin must be different")
	}

	for _, key := range []string{"Squelch_Active_High", "Ptt_Active_High", "RouteToBroadcast"} {
		if _, err := interfaceConfigBool(cfg.Entries, key); err != nil {
			return err
		}
	}

	return nil
}

func interfaceConfigString(entries []models.ConfigEntry, key string) (string, error) {
	for _, entry := range entries {
		if entry.Type != models.ApplicationTypeInterface || entry.Key != key {
			continue
		}
		value, ok := entry.Var.Data.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a string", key)
		}
		return value, nil
	}
	return "", fmt.Errorf("%s is missing", key)
}

func interfaceConfigInt(entries []models.ConfigEntry, key string) (int, error) {
	for _, entry := range entries {
		if entry.Type != models.ApplicationTypeInterface || entry.Key != key {
			continue
		}
		value, ok := entry.Var.Data.(int)
		if !ok {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return value, nil
	}
	return 0, fmt.Errorf("%s is missing", key)
}

func interfaceConfigBool(entries []models.ConfigEntry, key string) (bool, error) {
	for _, entry := range entries {
		if entry.Type != models.ApplicationTypeInterface || entry.Key != key {
			continue
		}
		value, ok := entry.Var.Data.(bool)
		if !ok {
			return false, fmt.Errorf("%s must be a boolean", key)
		}
		return value, nil
	}
	return false, fmt.Errorf("%s is missing", key)
}
