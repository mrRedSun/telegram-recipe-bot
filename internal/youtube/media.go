package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

func singleVideoMetadata(raw []byte) ([]byte, error) {
	var doc struct {
		Type    string            `json:"_type"`
		Entries []json.RawMessage `json:"entries"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil, errors.New("invalid source metadata")
	}
	if doc.Type == "playlist" || doc.Entries != nil {
		if len(doc.Entries) != 1 || string(doc.Entries[0]) == "null" {
			return nil, &UnsupportedError{Message: "Recipe extraction needs one public video or Reel, not a multi-video post."}
		}
		return singleVideoMetadata(doc.Entries[0])
	}
	return raw, nil
}
func mediaDuration(raw []byte, limit time.Duration) (float64, error) {
	var doc struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return 0, errors.New("invalid media inspection result")
	}
	duration, err := strconv.ParseFloat(doc.Format.Duration, 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, &UnsupportedError{Message: "The downloaded source is not a valid finite-duration video."}
	}
	if duration > limit.Seconds()+1 {
		return 0, &UnsupportedError{Message: fmt.Sprintf("That video is %.0f seconds; the limit is %.0f seconds.", duration, limit.Seconds())}
	}
	return duration, nil
}
func extractFrames(ctx context.Context, media, dir string, duration float64) ([]string, error) {
	framesDir := filepath.Join(dir, "frames")
	if err := os.Mkdir(framesDir, 0700); err != nil {
		return nil, err
	}
	interval := duration / 12
	if interval < 2 {
		interval = 2
	}
	if interval > 15 {
		interval = 15
	}
	filter := fmt.Sprintf("fps=1/%.3f,scale=768:-2:force_original_aspect_ratio=decrease", interval)
	args := []string{"-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-i", media, "-vf", filter, "-frames:v", "12", "-q:v", "5", filepath.Join(framesDir, "frame-%03d.jpg")}
	if _, err := runBounded(ctx, "frames", "ffmpeg", args, 64<<10); err != nil {
		return nil, err
	}
	frames, _ := filepath.Glob(filepath.Join(framesDir, "*.jpg"))
	sort.Strings(frames)
	if len(frames) == 0 {
		return nil, errors.New("video yielded no frames")
	}
	return frames, nil
}
