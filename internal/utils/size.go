package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses a size string like "1G", "500M", "10K" into bytes.
func ParseSize(sizeStr string) (int64, error) {
	if sizeStr == "" {
		return 0, nil
	}
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))
	multiplier := int64(1)
	
	switch {
	case strings.HasSuffix(sizeStr, "G") || strings.HasSuffix(sizeStr, "GB"):
		multiplier = 1024 * 1024 * 1024
		sizeStr = strings.TrimRight(sizeStr, "GB")
	case strings.HasSuffix(sizeStr, "M") || strings.HasSuffix(sizeStr, "MB"):
		multiplier = 1024 * 1024
		sizeStr = strings.TrimRight(sizeStr, "MB")
	case strings.HasSuffix(sizeStr, "K") || strings.HasSuffix(sizeStr, "KB"):
		multiplier = 1024
		sizeStr = strings.TrimRight(sizeStr, "KB")
	case strings.HasSuffix(sizeStr, "B"):
		sizeStr = strings.TrimRight(sizeStr, "B")
	}

	val, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format: %s", sizeStr)
	}
	return val * multiplier, nil
}

// FormatBytes converts a byte count into a human-readable string.
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
