// Package catalog retrieves reference recipes from a separately provisioned immutable database.
package catalog

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

//go:embed search.py
var searchPython string

type Query struct {
	Dish        string   `json:"dish"`
	Ingredients []string `json:"ingredients"`
}
type Entry struct {
	Title       string   `json:"title"`
	Ingredients []string `json:"ingredients"`
	Steps       []string `json:"steps"`
	URL         string   `json:"url"`
	Dataset     string   `json:"dataset"`
	License     string   `json:"license"`
	Score       float64  `json:"score"`
}
type Info struct {
	Records int    `json:"records"`
	Dataset string `json:"dataset"`
	License string `json:"license"`
}
type Searcher interface {
	Search(context.Context, Query) ([]Entry, error)
}
type Store struct {
	path string
	Info Info
}

func Open(ctx context.Context, path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("catalog path must be absolute")
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return nil, errors.New("catalog database is unavailable")
	}
	store := &Store{path: path}
	raw, err := store.call(ctx, "info", nil)
	if err != nil || json.Unmarshal(raw, &store.Info) != nil {
		return nil, errors.New("catalog database failed validation")
	}
	return store, nil
}
func (s *Store) Search(ctx context.Context, q Query) ([]Entry, error) {
	input, err := json.Marshal(q)
	if err != nil || len(input) > 16000 {
		return nil, errors.New("catalog query exceeds limit")
	}
	raw, err := s.call(ctx, "search", input)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	if json.Unmarshal(raw, &entries) != nil || len(entries) > 3 {
		return nil, errors.New("invalid catalog result")
	}
	return entries, nil
}

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (64 << 10) - b.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:max(remaining, 0)]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func (s *Store) call(parent context.Context, mode string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", "-c", searchPython, s.path, mode)
	cmd.Stdin = bytes.NewReader(input)
	out := &limitedBuffer{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil || out.overflow {
		return nil, errors.New("catalog lookup unavailable")
	}
	return out.Bytes(), nil
}
