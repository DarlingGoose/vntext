package tts

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNoVoicesLoaded     = errors.New("no voices loaded")
	ErrSpeakerNotFound    = errors.New("speaker not found")
	ErrSpeakerMissingClip = errors.New("speaker is missing reference audio clip")
	ErrSpeakerMissingText = errors.New("speaker is missing reference text")
	ErrEmptyText          = errors.New("text is empty")
	ErrNotInstalled       = errors.New("tts backend is not installed")
)

type TTS interface {
	LoadVoices(ctx context.Context, speakers ...Speaker) error
	Speak(ctx context.Context, text string, opts ...Option) (*Result, error)
	Voices() []Speaker
	Close() error
}

type Result struct {
	AudioPath string
	Stdout    string
	Stderr    string
	Duration  time.Duration
	Backend   string
	Speaker   string
}
