package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRequiredALSACommands(t *testing.T) {
	tests := []struct {
		name     string
		linkType string
		config   string
		want     []string
	}{
		{name: "interface", linkType: "interface", want: []string{"arecord", "aplay"}},
		{name: "radio broadcast", linkType: "broadcast", config: "[broadcast]\nType = RADIO\n", want: []string{"aplay"}},
		{name: "discord broadcast", linkType: "broadcast", config: "[broadcast]\nType = DISCORD\n", want: nil},
		{name: "listen", linkType: "listen", want: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &Manager{config: Config{LinkType: test.linkType}}
			if test.config != "" {
				manager.linkConfigPath = filepath.Join(t.TempDir(), "config.ini")
				if err := os.WriteFile(manager.linkConfigPath, []byte(test.config), 0o644); err != nil {
					t.Fatalf("write link config: %v", err)
				}
			}

			got, err := manager.requiredALSACommands()
			if err != nil {
				t.Fatalf("requiredALSACommands() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("requiredALSACommands() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMissingCommands(t *testing.T) {
	lookup := func(command string) (string, error) {
		if command == "missing" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + command, nil
	}

	if got, want := missingCommands([]string{"present", "missing"}, lookup), []string{"missing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("missingCommands() = %v, want %v", got, want)
	}
}
