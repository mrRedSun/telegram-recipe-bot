package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEditUsesNativeRichMessage(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/editMessageText" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer server.Close()

	client := New(server.URL, "token")
	if err := client.Edit(context.Background(), 7, 9, "<table><tr><td>Salt</td></tr></table>"); err != nil {
		t.Fatal(err)
	}
	rich, ok := payload["rich_message"].(map[string]any)
	if !ok {
		t.Fatalf("rich_message missing: %#v", payload)
	}
	if _, legacyTextPresent := payload["text"]; legacyTextPresent {
		t.Fatalf("legacy text field present: %#v", payload)
	}
	if rich["html"] != "<table><tr><td>Salt</td></tr></table>" || rich["skip_entity_detection"] != true {
		t.Fatalf("unexpected rich message: %#v", rich)
	}
}
