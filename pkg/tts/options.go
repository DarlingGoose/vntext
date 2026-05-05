package tts

import "time"

type Options struct {
	Speaker string
	Tone    string

	OutputDir  string
	OutputFile string

	Language       string
	Device         string
	DeviceFallback []string

	Speed         *float64
	RemoveSilence bool

	Timeout time.Duration

	// F5-TTS-specific knobs, but generic enough to keep here for now.
	Model             string
	ModelCfg          string
	CkptFile          string
	VocabFile         string
	VocoderName       string
	TargetRMS         *float64
	CrossFadeDuration *float64
	NFEStep           *int
	CFGStrength       *float64
	SwaySamplingCoef  *float64
	FixDuration       *float64

	Extra map[string]string
}

type Option func(*Options)

func NewOptions(opts ...Option) Options {
	o := Options{
		Model:          "F5TTS_v1_Base",
		VocoderName:    "vocos",
		Device:         "cpu",
		DeviceFallback: []string{"cpu"},
		Extra:          map[string]string{},
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return o
}

func WithSpeaker(speaker string) Option {
	return func(o *Options) {
		o.Speaker = speaker
	}
}

func WithDeviceFallback(devices ...string) Option {
	return func(o *Options) {
		o.DeviceFallback = append([]string(nil), devices...)
		if len(devices) > 0 {
			o.Device = devices[0]
		}
	}
}

func WithTone(tone string) Option {
	return func(o *Options) {
		o.Tone = tone
	}
}

func WithOutput(dir, file string) Option {
	return func(o *Options) {
		o.OutputDir = dir
		o.OutputFile = file
	}
}

func WithLanguage(language string) Option {
	return func(o *Options) {
		o.Language = language
	}
}

func WithDevice(device string) Option {
	return func(o *Options) {
		o.Device = device
	}
}

func WithSpeed(speed float64) Option {
	return func(o *Options) {
		o.Speed = &speed
	}
}

func WithRemoveSilence(enabled bool) Option {
	return func(o *Options) {
		o.RemoveSilence = enabled
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		o.Timeout = timeout
	}
}

func WithModel(model string) Option {
	return func(o *Options) {
		o.Model = model
	}
}

func WithModelCfg(path string) Option {
	return func(o *Options) {
		o.ModelCfg = path
	}
}

func WithCheckpoint(path string) Option {
	return func(o *Options) {
		o.CkptFile = path
	}
}

func WithVocab(path string) Option {
	return func(o *Options) {
		o.VocabFile = path
	}
}

func WithVocoder(name string) Option {
	return func(o *Options) {
		o.VocoderName = name
	}
}

func WithTargetRMS(v float64) Option {
	return func(o *Options) {
		o.TargetRMS = &v
	}
}

func WithCrossFadeDuration(v float64) Option {
	return func(o *Options) {
		o.CrossFadeDuration = &v
	}
}

func WithNFEStep(v int) Option {
	return func(o *Options) {
		o.NFEStep = &v
	}
}

func WithCFGStrength(v float64) Option {
	return func(o *Options) {
		o.CFGStrength = &v
	}
}

func WithSwaySamplingCoef(v float64) Option {
	return func(o *Options) {
		o.SwaySamplingCoef = &v
	}
}

func WithFixDuration(v float64) Option {
	return func(o *Options) {
		o.FixDuration = &v
	}
}

func WithExtra(key, value string) Option {
	return func(o *Options) {
		if o.Extra == nil {
			o.Extra = map[string]string{}
		}
		o.Extra[key] = value
	}
}
