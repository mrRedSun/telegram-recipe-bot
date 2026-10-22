// Package instagram defines the accepted public Instagram source URL policy.
package instagram

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var shortcodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{5,64}$`)
var urlPattern = regexp.MustCompile(`https?://[^\s<>]+`)

func Normalize(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return "", errors.New("invalid Instagram URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("Instagram URL must use HTTPS")
	}
	if u.User != nil {
		return "", errors.New("URL credentials are not allowed")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != "instagram.com" && host != "www.instagram.com" {
		return "", errors.New("only instagram.com post and Reel URLs are accepted")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || (parts[0] != "p" && parts[0] != "reel" && parts[0] != "reels") || !shortcodePattern.MatchString(parts[1]) {
		return "", errors.New("only direct public Instagram post and Reel URLs are accepted")
	}
	kind := parts[0]
	if kind == "reels" {
		kind = "reel"
	}
	return "https://www.instagram.com/" + kind + "/" + parts[1] + "/", nil
}

func Find(text string) (string, bool) {
	for _, candidate := range urlPattern.FindAllString(text, -1) {
		candidate = strings.TrimRight(candidate, ".,;:!?)]}\"'")
		if normalized, err := Normalize(candidate); err == nil {
			return normalized, true
		}
	}
	return "", false
}
