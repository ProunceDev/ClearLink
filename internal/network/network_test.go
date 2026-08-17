package network

import (
	"clearlink/internal/models"
	"testing"
)

func TestConfigFloatUpdateRoundTrip(t *testing.T) {
	packet := ToClientUpdateConfigEntryPacket{Entry: models.ConfigEntry{
		Key:  "CTCSS",
		Type: models.ApplicationTypeListen,
		Var: models.EntryVar{
			Type: models.EntryTypeFloat,
			Data: 123.0,
		},
		Default: models.EntryVar{
			Type: models.EntryTypeFloat,
			Data: 0.0,
		},
	}}

	payload, err := packet.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := UnmarshalToClientUpdateConfigEntry(payload)
	if err != nil {
		t.Fatalf("UnmarshalToClientUpdateConfigEntry() error = %v", err)
	}
	if got, ok := decoded.Entry.Var.Data.(float64); !ok || got != 123.0 {
		t.Fatalf("decoded float config value = %#v, want float64(123)", decoded.Entry.Var.Data)
	}
}

func TestAudioChunkPacketRejectsPayloadsLargerThanHeaderLength(t *testing.T) {
	maximum := ToAnyAudioChunkPacket{Samples: make([]int16, MaxAudioChunkSamples)}
	if _, err := maximum.Marshal(); err != nil {
		t.Fatalf("maximum-size audio chunk failed to marshal: %v", err)
	}

	overflow := ToAnyAudioChunkPacket{Samples: make([]int16, MaxAudioChunkSamples+1)}
	if _, err := overflow.Marshal(); err == nil {
		t.Fatal("oversized audio chunk marshaled successfully")
	}
}
