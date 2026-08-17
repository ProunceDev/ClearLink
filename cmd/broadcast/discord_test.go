package main

import "testing"

func TestLinearPCMResamplerPreserves48KHzSamples(t *testing.T) {
	resampler := linearPCMResampler{}
	input := []int16{-100, 0, 100, 32767, -32768}
	output := resampler.process(input, 48000)
	if len(output) != len(input) {
		t.Fatalf("output length = %d, want %d", len(output), len(input))
	}
	for index := range input {
		if output[index] != input[index] {
			t.Fatalf("output[%d] = %d, want %d", index, output[index], input[index])
		}
	}
}

func TestLinearPCMResamplerConverts16KHzDurationTo48KHz(t *testing.T) {
	resampler := linearPCMResampler{}
	input := make([]int16, 320)
	for index := range input {
		input[index] = int16(index * 10)
	}

	var output []int16
	for chunk := 0; chunk < 4; chunk++ {
		output = append(output, resampler.process(input, 16000)...)
	}
	if difference := len(output) - 3840; difference < -3 || difference > 0 {
		t.Fatalf("resampled sample count = %d, want 3837 through 3840", len(output))
	}
	if len(output) == 0 || output[len(output)-1] <= output[0] {
		t.Fatal("resampler did not preserve the input signal direction")
	}
}
