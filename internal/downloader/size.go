package downloader

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses a human size like "500", "10KB", "5MB", "1GB".
// Plain numbers are treated as bytes. Empty or "0" means unlimited (0).
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" || s == "0" {
		return 0, nil
	}

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "KB"), strings.HasSuffix(s, "K"):
		multiplier = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K")
	case strings.HasSuffix(s, "MB"), strings.HasSuffix(s, "M"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	case strings.HasSuffix(s, "GB"), strings.HasSuffix(s, "G"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	case strings.HasSuffix(s, "B"):
		s = strings.TrimSuffix(s, "B")
	}
	s = strings.TrimSpace(s)

	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid size %q (try 10MB, 500KB, or bytes)", s)
	}
	return int64(n * float64(multiplier)), nil
}
