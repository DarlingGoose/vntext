package util

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	versionRegex = regexp.MustCompile(`(([vV](er){0,1}(sion){0,1})[0-9\.\-]+)`)
	sepRegex     = regexp.MustCompile(`[_\-]`)
	titleCaser   = cases.Title(language.English)
)

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func DeriveGameName(inputPath, executablePath string, inputWasDir bool) string {
	var name string
	if inputWasDir {
		name = filepath.Base(inputPath)
	} else {
		name = strings.TrimSuffix(filepath.Base(executablePath), filepath.Ext(executablePath))
	}

	// Clean name
	name = versionRegex.ReplaceAllString(name, "")
	name = sepRegex.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	// Title-case only if it looks like English
	if isMostlyASCII(name) {
		name = titleCaser.String(strings.ToLower(name))
	}

	return name
}

func SanitizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "_", "-")
	name = replacer.Replace(name)
	var builder strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		}
	}
	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		return "game"
	}
	return sanitized
}

func isMostlyASCII(s string) bool {
	if s == "" {
		return false
	}

	var asciiCount, total int
	for _, r := range s {
		if unicode.IsLetter(r) {
			total++
			if r <= unicode.MaxASCII {
				asciiCount++
			}
		}
	}

	// avoid division by zero
	if total == 0 {
		return false
	}

	// threshold: 80% ASCII letters
	return float64(asciiCount)/float64(total) > 0.8
}

func ByteCountIEC(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	// "KMGTPE" represents Kilo, Mega, Giga, Tera, Peta, Exa
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
