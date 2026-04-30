package sailor

import (
	"fmt"
	"os"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const (
	ansiReset   = "\033[0m"
	ansiBold    = "\033[1m"
	ansiDim     = "\033[2m"
	ansiRed     = "\033[31m"
	ansiGreen   = "\033[32m"
	ansiYellow  = "\033[33m"
	ansiBlue    = "\033[34m"
	ansiMagenta = "\033[35m"
	ansiCyan    = "\033[36m"
)

var devPrefix = ansiBold + ansiCyan + "[dev]" + ansiReset

// devLogLine prints a single structured DEV log line to stderr.
//
//	[dev]  <kind>    <icon> <label>   <detail>
func devLogLine(kind opts.ResourceKind, icon, label, labelColor, detail string) {
	var kindPart string
	if kind != "" {
		kindPart = fmt.Sprintf("%s%-8s%s", ansiYellow, string(kind), ansiReset)
	} else {
		kindPart = "        " // 8 spaces — aligns with padded kind column
	}

	statusPart := fmt.Sprintf("%s%s %-10s%s", labelColor, icon, label, ansiReset)
	detailPart := fmt.Sprintf("%s%s%s", ansiDim, detail, ansiReset)

	fmt.Fprintf(os.Stderr, "  %s  %s  %s  %s\n",
		devPrefix, kindPart, statusPart, detailPart)
}

func devLogBanner(env, host, ns, app string) {
	line := ansiCyan + "─────────────────────────────────────────────────────────────────────" + ansiReset
	fmt.Fprintf(os.Stderr, "\n  %s┌── sailor %s\n", ansiCyan, line+ansiReset)
	fmt.Fprintf(os.Stderr, "  %s│%s  env   %s%s%s  →  %s%s%s\n",
		ansiCyan, ansiReset, ansiBold, env, ansiReset, ansiDim, host, ansiReset)
	fmt.Fprintf(os.Stderr, "  %s│%s  app   %s%s / %s%s\n",
		ansiCyan, ansiReset, ansiBold, ns, app, ansiReset)
	fmt.Fprintf(os.Stderr, "  %s└%s\n\n", ansiCyan, line+ansiReset)
}

func devLogCacheHit(kind opts.ResourceKind, path string) {
	devLogLine(kind, "●", "cache hit", ansiBold+ansiGreen, path)
}

func devLogFetching(kind opts.ResourceKind, url string) {
	devLogLine(kind, "↓", "fetching", ansiBold+ansiBlue, url)
}

func devLogCached(kind opts.ResourceKind, path string) {
	devLogLine(kind, "✓", "cached", ansiGreen, path)
}

func devLogWatching(kind opts.ResourceKind, dir string) {
	devLogLine(kind, "→", "watching", ansiBold+ansiMagenta, dir)
}

func devLogReloaded(kind opts.ResourceKind, path string) {
	devLogLine(kind, "↺", "reloaded", ansiBold+ansiCyan, path)
}

func devLogReloadError(kind opts.ResourceKind, detail string) {
	devLogLine(kind, "✕", "reload err", ansiBold+ansiRed, detail)
}
