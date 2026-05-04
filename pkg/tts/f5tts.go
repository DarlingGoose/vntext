package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultF5Binary = "f5-tts_infer-cli"
	BackendF5TTS    = "f5-tts"
)

type F5TTS struct {
	binary string

	mu       sync.RWMutex
	speakers map[string]Speaker

	defaultOptions []Option
}

type F5Config struct {
	Binary         string
	DefaultOptions []Option
}

func NewF5(cfg F5Config) (*F5TTS, error) {
	bin := strings.TrimSpace(cfg.Binary)
	if bin == "" {
		bin = DefaultF5Binary
	}

	t := &F5TTS{
		binary:         bin,
		speakers:       map[string]Speaker{},
		defaultOptions: slices.Clone(cfg.DefaultOptions),
	}

	if err := t.CheckInstalled(); err != nil {
		return nil, err
	}

	return t, nil
}

func NewF5Unchecked(cfg F5Config) *F5TTS {
	bin := strings.TrimSpace(cfg.Binary)
	if bin == "" {
		bin = DefaultF5Binary
	}

	return &F5TTS{
		binary:         bin,
		speakers:       map[string]Speaker{},
		defaultOptions: slices.Clone(cfg.DefaultOptions),
	}
}

func (f *F5TTS) CheckInstalled() error {
	if filepath.IsAbs(f.binary) {
		info, err := os.Stat(f.binary)
		if err != nil || info.IsDir() {
			return fmt.Errorf("%w: %s. Install with `yay -S f5-tts` or see https://github.com/SWivid/F5-TTS", ErrNotInstalled, f.binary)
		}
		return nil
	}

	if _, err := exec.LookPath(f.binary); err != nil {
		return fmt.Errorf("%w: %s. Install with `yay -S f5-tts` or see https://github.com/SWivid/F5-TTS", ErrNotInstalled, f.binary)
	}

	return nil
}

func (f *F5TTS) LoadVoices(ctx context.Context, speakers ...Speaker) error {
	_ = ctx

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.speakers == nil {
		f.speakers = map[string]Speaker{}
	}

	for _, speaker := range speakers {
		if strings.TrimSpace(speaker.ID) == "" {
			if strings.TrimSpace(speaker.Name) != "" {
				speaker.ID = normalizeID(speaker.Name)
			} else {
				return fmt.Errorf("speaker ID or name is required")
			}
		}

		if len(speaker.VoiceClipsPath) == 0 {
			return fmt.Errorf("%w: %s", ErrSpeakerMissingClip, speaker.ID)
		}

		if strings.TrimSpace(speaker.ReferenceText) == "" {
			return fmt.Errorf("%w: %s", ErrSpeakerMissingText, speaker.ID)
		}

		f.speakers[speaker.ID] = speaker
	}

	return nil
}

func (f *F5TTS) Voices() []Speaker {
	f.mu.RLock()
	defer f.mu.RUnlock()

	out := make([]Speaker, 0, len(f.speakers))
	for _, speaker := range f.speakers {
		out = append(out, speaker)
	}

	slices.SortFunc(out, func(a, b Speaker) int {
		return strings.Compare(a.ID, b.ID)
	})

	return out
}

func (f *F5TTS) Speak(ctx context.Context, text string, opts ...Option) (*Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ErrEmptyText
	}

	if err := f.CheckInstalled(); err != nil {
		return nil, err
	}

	allOpts := make([]Option, 0, len(f.defaultOptions)+len(opts))
	allOpts = append(allOpts, f.defaultOptions...)
	allOpts = append(allOpts, opts...)

	o := NewOptions(allOpts...)

	speaker, err := f.resolveSpeaker(o.Speaker)
	if err != nil {
		return nil, err
	}

	if o.Timeout > 0 {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, o.Timeout)
			defer cancel()
		}
	}

	args := f.buildArgs(text, speaker, o)

	cmd := exec.CommandContext(ctx, f.binary, args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(start)

	result := &Result{
		AudioPath: expectedOutputPath(o),
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Duration:  elapsed,
		Backend:   BackendF5TTS,
		Speaker:   speaker.ID,
	}

	if runErr != nil {
		return result, fmt.Errorf("run %s: %w: %s", f.binary, runErr, strings.TrimSpace(stderr.String()))
	}

	return result, nil
}

func (f *F5TTS) Close() error {
	return nil
}

func (f *F5TTS) resolveSpeaker(id string) (Speaker, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if len(f.speakers) == 0 {
		return Speaker{}, ErrNoVoicesLoaded
	}

	id = strings.TrimSpace(id)
	if id != "" {
		speaker, ok := f.speakers[id]
		if !ok {
			return Speaker{}, fmt.Errorf("%w: %s", ErrSpeakerNotFound, id)
		}
		return speaker, nil
	}

	if len(f.speakers) == 1 {
		for _, speaker := range f.speakers {
			return speaker, nil
		}
	}

	return Speaker{}, fmt.Errorf("%w: no speaker selected", ErrSpeakerNotFound)
}

func (f *F5TTS) buildArgs(text string, speaker Speaker, o Options) []string {
	var args []string

	add := func(flag string, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		args = append(args, flag, value)
	}

	add("-m", o.Model)
	add("-mc", o.ModelCfg)
	add("-p", o.CkptFile)
	add("-v", o.VocabFile)

	add("-r", speaker.BestReferenceAudio())
	add("-s", speaker.ReferenceText)
	add("-t", text)

	add("-o", o.OutputDir)
	add("-w", o.OutputFile)

	if o.RemoveSilence {
		args = append(args, "--remove_silence")
	}

	add("--vocoder_name", o.VocoderName)
	add("--device", o.Device)

	addFloat(&args, "--speed", o.Speed)
	addFloat(&args, "--target_rms", o.TargetRMS)
	addFloat(&args, "--cross_fade_duration", o.CrossFadeDuration)
	addInt(&args, "--nfe_step", o.NFEStep)
	addFloat(&args, "--cfg_strength", o.CFGStrength)
	addFloat(&args, "--sway_sampling_coef", o.SwaySamplingCoef)
	addFloat(&args, "--fix_duration", o.FixDuration)

	return args
}

func expectedOutputPath(o Options) string {
	if strings.TrimSpace(o.OutputDir) == "" || strings.TrimSpace(o.OutputFile) == "" {
		return ""
	}
	return filepath.Join(o.OutputDir, o.OutputFile)
}

func addFloat(args *[]string, flag string, value *float64) {
	if value == nil {
		return
	}
	*args = append(*args, flag, strconv.FormatFloat(*value, 'f', -1, 64))
}

func addInt(args *[]string, flag string, value *int) {
	if value == nil {
		return
	}
	*args = append(*args, flag, strconv.Itoa(*value))
}

func normalizeID(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

var _ TTS = (*F5TTS)(nil)

func IsNotInstalled(err error) bool {
	return errors.Is(err, ErrNotInstalled)
}
