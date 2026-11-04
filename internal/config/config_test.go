package config

import (
	"clearlink/internal/models"
	"testing"

	"gopkg.in/ini.v1"
)

func TestConfigEntryDefaultValueMigratesLegacyListenSettings(t *testing.T) {
	file := ini.Empty()
	section := file.Section("listen")
	section.Key("SquelchDB").SetValue("-32")
	section.Key("AudioCutoffHz").SetValue("2800")
	section.Key("DeemphasisTauUs").SetValue("75")
	section.Key("TunerBandwidth").SetValue("12500")

	for _, test := range []struct {
		key  string
		want string
	}{
		{key: "SquelchThreshold", want: "-32"},
		{key: "Lowpass", want: "2800"},
		{key: "Tau", want: "75"},
		{key: "Bandwidth", want: "0"},
	} {
		entry := models.ConfigEntry{
			Key:  test.key,
			Type: models.ApplicationTypeListen,
			Default: models.EntryVar{
				Type: models.EntryTypeInt,
				Data: 0,
			},
		}
		if got := configEntryDefaultValue(entry, section); got != test.want {
			t.Errorf("configEntryDefaultValue(%q) = %q, want %q", test.key, got, test.want)
		}
	}
}

func TestConfigEntryDefaultValueFormatsFloat(t *testing.T) {
	entry := models.ConfigEntry{
		Key:  "NotchQ",
		Type: models.ApplicationTypeListen,
		Default: models.EntryVar{
			Type: models.EntryTypeFloat,
			Data: 10.5,
		},
	}
	if got := configEntryDefaultValue(entry, ini.Empty().Section("listen")); got != "10.5" {
		t.Errorf("configEntryDefaultValue() = %q, want %q", got, "10.5")
	}
}

func TestConfigEntryDefaultValueFormatsBool(t *testing.T) {
	entry := models.ConfigEntry{
		Key:  "RouteToBroadcast",
		Type: models.ApplicationTypeInterface,
		Default: models.EntryVar{
			Type: models.EntryTypeBool,
			Data: true,
		},
	}
	if got := configEntryDefaultValue(entry, ini.Empty().Section("interface")); got != "true" {
		t.Errorf("configEntryDefaultValue() = %q, want %q", got, "true")
	}
}

func TestListenCTCSSDebounceDefault(t *testing.T) {
	for _, entry := range ConfigEntries {
		if entry.Type != models.ApplicationTypeListen || entry.Key != "CTCSSDebounceMS" {
			continue
		}
		if got, want := entry.Default.Data, 50; got != want {
			t.Fatalf("CTCSSDebounceMS default = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("CTCSSDebounceMS configuration entry is missing")
}

func TestBroadcastAudioOutputGainDefault(t *testing.T) {
	for _, entry := range ConfigEntries {
		if entry.Type != models.ApplicationTypeBroadcast || entry.Key != "AudioOutputGain" {
			continue
		}
		if got, want := entry.Default.Data, 1.0; got != want {
			t.Fatalf("AudioOutputGain default = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("AudioOutputGain configuration entry is missing")
}

func TestBroadcastHangTimeDefault(t *testing.T) {
	for _, entry := range ConfigEntries {
		if entry.Type != models.ApplicationTypeBroadcast || entry.Key != "HangTimeMS" {
			continue
		}
		if got, want := entry.Default.Data, 50; got != want {
			t.Fatalf("HangTimeMS default = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("HangTimeMS configuration entry is missing")
}

func TestValidateInterfaceConfig(t *testing.T) {
	cfg := defaultInterfaceConfig()
	if err := ValidateInterfaceConfig(cfg); err != nil {
		t.Fatalf("ValidateInterfaceConfig() error = %v", err)
	}

	for index := range cfg.Entries {
		if cfg.Entries[index].Key == "AudioRate" {
			cfg.Entries[index].Var.Data = 44100
		}
		if cfg.Entries[index].Key == "AudioChunkMs" {
			cfg.Entries[index].Var.Data = 7
		}
	}
	if err := ValidateInterfaceConfig(cfg); err == nil {
		t.Fatal("ValidateInterfaceConfig() accepted a non-integral audio frame")
	}

	cfg = defaultInterfaceConfig()
	for index := range cfg.Entries {
		if cfg.Entries[index].Key == "Ptt_Pin" {
			cfg.Entries[index].Var.Data = 17
		}
	}
	if err := ValidateInterfaceConfig(cfg); err == nil {
		t.Fatal("ValidateInterfaceConfig() accepted matching GPIO pins")
	}
}

func defaultInterfaceConfig() models.Config {
	cfg := models.Config{}
	for _, entry := range ConfigEntries {
		if entry.Type != models.ApplicationTypeInterface {
			continue
		}
		entry.Var = entry.Default
		cfg.Entries = append(cfg.Entries, entry)
	}
	return cfg
}
