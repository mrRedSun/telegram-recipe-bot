package catalog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "input.jsonl")
	path := filepath.Join(dir, "catalog.sqlite")
	data := `{"title":"Chicken lemon soup","ingredients":["2 cups chicken","1 lemon","2 carrots"],"steps":["Simmer chicken and carrots, then add lemon."],"url":"https://example.com/chicken"}
{"title":"Chocolate brownies","ingredients":["flour","sugar","eggs","chocolate"],"steps":["Bake."],"url":"https://example.com/brownies"}
`
	if err := os.WriteFile(input, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "../../tools/catalog/build.py", "--format", "jsonl", "--dataset", "Test fixtures", "--license", "CC0-1.0", "--input", input, "--output", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture import: %v %s", err, out)
	}
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
func TestRetrievalRelevanceAndAbstention(t *testing.T) {
	store := fixture(t)
	for _, tc := range []struct {
		name  string
		query Query
		want  int
	}{
		{"dish and observed ingredients", Query{Dish: "chicken lemon soup", Ingredients: []string{"chicken", "lemon", "carrot"}}, 1},
		{"unrelated ingredients", Query{Dish: "beef stew", Ingredients: []string{"beef", "potato", "tomato"}}, 0},
		{"pantry staples", Query{Ingredients: []string{"salt", "flour", "eggs", "sugar"}}, 0},
		{"no identifiable food", Query{}, 0},
		{"operators are plain text", Query{Dish: `" OR title:* NOT recipes`}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.Search(context.Background(), tc.query)
			if err != nil || len(got) != tc.want {
				t.Fatalf("matches=%d err=%v", len(got), err)
			}
		})
	}
	if store.Info.Records != 2 {
		t.Fatal("incorrect catalogue metadata")
	}
}
func TestReadOnlyConcurrentQueriesAndPrivateErrors(t *testing.T) {
	store := fixture(t)
	before, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if _, err := store.Search(context.Background(), Query{Dish: "chicken soup"}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	after, _ := os.ReadFile(store.path)
	if string(after) != string(before) {
		t.Fatal("search mutated database")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Search(ctx, Query{Dish: "private phrase"}); err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), store.path) {
		t.Fatalf("unsafe error: %v", err)
	}
	if _, err := Open(context.Background(), "relative.sqlite"); err == nil {
		t.Fatal("accepted relative path")
	}
}
func TestRejectInvalidCatalogue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.sqlite")
	_ = os.WriteFile(path, []byte("not a database"), 0600)
	if _, err := Open(context.Background(), path); err == nil || strings.Contains(err.Error(), path) {
		t.Fatal("invalid database was accepted or exposed")
	}
}
