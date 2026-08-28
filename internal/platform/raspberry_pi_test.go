package platform

import "testing"

func TestIsRaspberryPiModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{model: "Raspberry Pi 3 Model B Plus Rev 1.3\x00", want: true},
		{model: "Raspberry Pi 5 Model B Rev 1.0", want: true},
		{model: "Raspberry Pi Zero 2 W Rev 1.0", want: true},
		{model: "Generic x86 PC", want: false},
		{model: "", want: false},
	}

	for _, test := range tests {
		if got := IsRaspberryPiModel(test.model); got != test.want {
			t.Errorf("IsRaspberryPiModel(%q) = %t, want %t", test.model, got, test.want)
		}
	}
}
