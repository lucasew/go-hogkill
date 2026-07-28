package render

import (
	"fmt"
	"math"
	"strings"
)

// Bytes formats a byte count.
func Bytes(value uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	n := float64(value)
	unit := 0
	for n >= 1024 && unit < len(units)-1 {
		n /= 1024
		unit++
	}
	digits := 0
	if unit != 0 {
		if n < 10 {
			digits = 2
		} else if n < 100 {
			digits = 1
		}
	}
	return fmt.Sprintf("%.*f %s", digits, n, units[unit])
}

// Percent formats a CPU-like percentage.
func Percent(value float64) string {
	if value >= 100 {
		return fmt.Sprintf("%.0f%%", value)
	}
	return fmt.Sprintf("%.1f%%", value)
}

// Duration formats elapsed seconds.
func Duration(seconds float64) string {
	s := int(math.Round(seconds))
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	if s < 3600 {
		return fmt.Sprintf("%dm", s/60)
	}
	if s < 86400 {
		return fmt.Sprintf("%dh%dm", s/3600, (s%3600)/60)
	}
	return fmt.Sprintf("%dd%dh", s/86400, (s%86400)/3600)
}

var blocks = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// Bar fractional unicode bar of width columns.
func Bar(ratio float64, width int) string {
	if width <= 0 {
		return ""
	}
	clamped := math.Max(0, math.Min(1, ratio))
	exact := clamped * float64(width)
	full := int(math.Floor(exact))
	rem := int(math.Floor((exact - float64(full)) * 8))
	filled := strings.Repeat("█", full)
	if full < width && rem > 0 && rem < len(blocks) {
		filled += blocks[rem]
	}
	return padRight(filled, width)
}

func padRight(s string, width int) string {
	n := len([]rune(s))
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// Fit truncates/pads to width (runes, no ANSI).
func Fit(text string, width int) string {
	r := []rune(text)
	if width <= 0 {
		return ""
	}
	if len(r) > width {
		if width == 1 {
			return string(r[:1])
		}
		return string(r[:width-1]) + "…"
	}
	return string(r) + strings.Repeat(" ", width-len(r))
}

// PadStart left-pads to width.
func PadStart(text string, width int) string {
	r := []rune(text)
	if len(r) >= width {
		return text
	}
	return strings.Repeat(" ", width-len(r)) + text
}
