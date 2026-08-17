package config

import (
	"clearlink/internal/models"
	"fmt"
	"os"

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
	// LISTEN
	{Key: "AuthKey", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeString, Data: "Default"}},
	{Key: "ServerPort", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 4125}},
	{Key: "ServerAddr", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeString, Data: "127.0.0.1"}},
	{Key: "NodeName", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeString, Data: "DefaultNodeName"}},
	{Key: "Frequency", Type: models.ApplicationTypeListen, Default: models.EntryVar{Type: models.EntryTypeInt, Data: 146520000}},
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
				}
			}

			cfg.SaveTo(FilePath)
			return
		}
	}
}
