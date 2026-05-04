package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ScanOptions struct {
	Root string

	// If true, attempts to get duration with ffprobe.
	// Requires ffprobe to be installed and available in PATH.
	IncludeDuration bool

	// Optional override. Defaults to: mp3, wav, ogg, flac, m4a, aac, opus, wma
	Extensions map[string]bool
}

type AudioGroup struct {
	Name  string      `json:"name"`
	Files []AudioFile `json:"files"`
}

type AudioFile struct {
	Path         string        `json:"path"`
	Group        string        `json:"group"`
	SizeBytes    int64         `json:"size_bytes"`
	Duration     time.Duration `json:"duration,omitempty"`
	DurationText string        `json:"duration_text,omitempty"`
}

func FindGroupedAudioFiles(ctx context.Context, opts ScanOptions) ([]AudioGroup, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}

	exts := opts.Extensions
	if len(exts) == 0 {
		exts = defaultAudioExts()
	}

	trailingDigits := regexp.MustCompile(`\d+$`)
	grouped := map[string][]AudioFile{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Skip unreadable paths but keep scanning.
			return nil
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !exts[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		group := trailingDigits.ReplaceAllString(base, "")
		if group == "" {
			group = base
		}

		file := AudioFile{
			Path:      path,
			Group:     group,
			SizeBytes: info.Size(),
		}

		if opts.IncludeDuration {
			duration, err := ProbeAudioDuration(ctx, path)
			if err == nil {
				file.Duration = duration
				file.DurationText = duration.Round(time.Millisecond).String()
			}
		}

		grouped[group] = append(grouped[group], file)
		return nil
	})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)

	groups := make([]AudioGroup, 0, len(names))
	for _, name := range names {
		files := grouped[name]
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})

		groups = append(groups, AudioGroup{
			Name:  name,
			Files: files,
		})
	}

	return groups, nil
}

func ProbeAudioDuration(ctx context.Context, path string) (time.Duration, error) {
	cmd := exec.CommandContext(
		ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffprobe %q: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}

	var out struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return 0, err
	}

	seconds, err := strconv.ParseFloat(out.Format.Duration, 64)
	if err != nil {
		return 0, err
	}

	return time.Duration(seconds * float64(time.Second)), nil
}

func defaultAudioExts() map[string]bool {
	return map[string]bool{
		".mp3":  true,
		".wav":  true,
		".ogg":  true,
		".flac": true,
		".m4a":  true,
		".aac":  true,
		".opus": true,
		".wma":  true,
	}
}
