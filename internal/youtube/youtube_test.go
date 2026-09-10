package youtube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{"https://youtube.com/shorts/abcDEF_1234": "https://www.youtube.com/watch?v=abcDEF_1234", "https://youtu.be/abcDEF_1234?t=4": "https://www.youtube.com/watch?v=abcDEF_1234", "https://www.youtube.com/watch?v=abcDEF_1234": "https://www.youtube.com/watch?v=abcDEF_1234"}
	for in, want := range cases {
		got, err := Normalize(in)
		if err != nil || got != want {
			t.Errorf("Normalize(%q)=%q,%v", in, got, err)
		}
	}
}
func TestNormalizeRejectsSSRF(t *testing.T) {
	for _, in := range []string{"http://127.0.0.1/x", "file:///etc/passwd", "https://youtube.com.evil.test/shorts/abcDEF_1234", "https://youtube.com/playlist?list=abc"} {
		if _, err := Normalize(in); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestReadAuthorComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "comments.info.json")
	data := `{"comments":[{"text":"viewer guess","author_is_uploader":false,"is_pinned":true},{"text":"Bake for 20 minutes","author_is_uploader":true,"is_pinned":false},{"text":"Ingredients are in the description","author_is_uploader":true,"is_pinned":true},{"text":"Bake for 20 minutes","author_is_uploader":true,"is_pinned":false}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readAuthorComments(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "viewer guess") {
		t.Fatalf("included viewer comment: %q", got)
	}
	if strings.Count(got, "Bake for 20 minutes") != 1 {
		t.Fatalf("did not deduplicate: %q", got)
	}
	if !strings.HasPrefix(got, "[Pinned author comment]\nIngredients are in the description") {
		t.Fatalf("pinned comment not first: %q", got)
	}
}
