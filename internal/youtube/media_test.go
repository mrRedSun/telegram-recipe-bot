package youtube

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSingleVideoPlaylistMetadataPreservesCreatorCaption(t *testing.T) {
	selected, err := singleVideoMetadata([]byte(`{"_type":"playlist","entries":[{"title":"Soup","description":"Add flour"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if json.Unmarshal(selected, &got) != nil || got["description"] != "Add flour" {
		t.Fatal("lost creator caption")
	}
	for _, raw := range []string{`{"_type":"playlist","entries":[]}`, `{"_type":"playlist","entries":[null]}`, `{"entries":[{},{}]}`} {
		if _, err := singleVideoMetadata([]byte(raw)); err == nil {
			t.Fatal("accepted missing or multiple videos")
		}
	}
}
func TestActualDurationEnforcesLimitWhenMetadataIsMissing(t *testing.T) {
	for _, duration := range []string{"0", "-1", "NaN", "+Inf", "900"} {
		_, err := mediaDuration([]byte(`{"format":{"duration":"`+duration+`"}}`), 4*time.Minute)
		var unsupported *UnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("accepted unsupported actual duration %s: %v", duration, err)
		}
	}
	if got, err := mediaDuration([]byte(`{"format":{"duration":"24.5"}}`), time.Minute); err != nil || got != 24.5 {
		t.Fatalf("valid Reel duration: %v,%v", got, err)
	}
}
func TestCommandOutputBufferBoundsMemory(t *testing.T) {
	b := &boundedBuffer{Limit: 16}
	input := []byte(strings.Repeat("x", 128))
	n, err := b.Write(input)
	if err != nil || n != 128 || b.Len() != 16 || !b.Overflow {
		t.Fatal("output was not bounded")
	}
}
