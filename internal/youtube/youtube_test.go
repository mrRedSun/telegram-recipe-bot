package youtube

import "testing"

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
