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
