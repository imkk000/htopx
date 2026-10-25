package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/imkk000/htopx/internal/sysinfo"
)

const (
	cBlack     = "30"
	cRed       = "31"
	cGreen     = "32"
	cYellow    = "33"
	cBlue      = "34"
	cMagenta   = "35"
	cCyan      = "36"
	cBold      = "1"
	cDim       = "1;90"
	cBoldCyan  = "1;36"
	cBoldGreen = "1;32"
	cOnGreen   = "30;42"
	cOnCyan    = "30;46"
	cOnBlue    = "30;44"
	cBoldRed   = "1;31"
)

type painter func(code, s string) string

func paint(code, s string) string {
	if s == "" {
		return ""
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func plain(_, s string) string { return s }

func padLeft(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}

func padRight(s string, w int) string {
	if n := ansi.StringWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return padRight(ansi.Truncate(s, w, ""), w)
}

func humanKiB(k uint64) string {
	units := []string{"K", "M", "G", "T", "P"}
	v := float64(k)
	var i int
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	switch {
	case v < 10:
		return fmt.Sprintf("%.2f%s", v, units[i])
	case v < 100:
		return fmt.Sprintf("%.1f%s", v, units[i])
	default:
		return fmt.Sprintf("%.0f%s", v, units[i])
	}
}

func kbytesCell(k uint64, p painter) string {
	const w = 5
	if k < 100000 {
		s := strconv.FormatUint(k, 10)
		if len(s) <= 3 {
			return padLeft(s, w)
		}
		cut := len(s) - 3
		return strings.Repeat(" ", w-len(s)) + p(cCyan, s[:cut]) + s[cut:]
	}
	mib := k / 1024
	if mib < 10000 {
		s := strconv.FormatUint(mib, 10)
		pad := strings.Repeat(" ", w-len(s)-1)
		if len(s) <= 3 {
			return pad + p(cCyan, s+"M")
		}
		cut := len(s) - 3
		return pad + p(cGreen, s[:cut]) + p(cCyan, s[cut:]+"M")
	}
	gib := float64(k) / 1024 / 1024
	if gib < 100 {
		s := fmt.Sprintf("%.1f", gib)
		dot := strings.IndexByte(s, '.')
		return padLeft(p(cGreen, s[:dot])+p(cCyan, s[dot:])+p(cGreen, "G"), w)
	}
	if gib < 100000 {
		return padLeft(p(cGreen, fmt.Sprintf("%.0fG", gib)), w)
	}
	return padLeft(p(cGreen, fmt.Sprintf("%.0fT", gib/1024)), w)
}

func timeCell(ticks uint64) string {
	cs := ticks * 100 / sysinfo.ClockTicks
	secs := cs / 100
	h, m, s := secs/3600, secs/60%60, secs%60
	if h > 0 {
		return padLeft(fmt.Sprintf("%dh%02d:%02d", h, m, s), 9)
	}
	return padLeft(fmt.Sprintf("%d:%02d.%02d", m, s, cs%100), 9)
}

func formatUptime(secs int64) string {
	days := secs / 86400
	clock := fmt.Sprintf("%02d:%02d:%02d", secs/3600%24, secs/60%60, secs%60)
	switch days {
	case 0:
		return clock
	case 1:
		return "1 day, " + clock
	default:
		return fmt.Sprintf("%d days, %s", days, clock)
	}
}
