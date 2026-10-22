package instagram

import "testing"

func TestNormalizeAndFind(t *testing.T) {
	got, err := Normalize("https://instagram.com/reels/Abc_123/?utm_source=test")
	if err != nil || got != "https://www.instagram.com/reel/Abc_123/" {
		t.Fatalf("Normalize() = %q, %v", got, err)
	}
	if found, ok := Find("please fetch https://www.instagram.com/p/Abc-123/ thanks"); !ok || found != "https://www.instagram.com/p/Abc-123/" {
		t.Fatalf("Find() = %q, %v", found, ok)
	}
}

func TestNormalizeRejectsProfilesAndLookalikes(t *testing.T) {
	for _, raw := range []string{
		"https://instagram.com/example/",
		"https://instagram.example/p/Abc123/",
		"https://user:pass@instagram.com/p/Abc123/",
		"file:///tmp/media",
	} {
		if _, err := Normalize(raw); err == nil {
			t.Fatalf("Normalize(%q) unexpectedly succeeded", raw)
		}
	}
}
