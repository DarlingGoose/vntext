package engine

import (
	"mime"
	"path/filepath"
	"strings"
)

type EngineFileInfo struct {
	Name       string
	Data       []byte
	MediaType  string // audio/ogg, audio/wav, audio/mpeg
	Ext        string // .ogg, .wav, .mp3
	SampleRate int
	Channels   int
	BitDepth   int
}

func NewEngineFileInfo(path string, data []byte) *EngineFileInfo {
	ext := strings.ToLower(filepath.Ext(path))
	mediaType := mime.TypeByExtension(ext)
	if idx := strings.Index(mediaType, ";"); idx >= 0 {
		mediaType = mediaType[:idx]
	}
	if mediaType == "" {
		mediaType = mediaTypeForExt(ext)
	}

	return &EngineFileInfo{
		Name:      filepath.Base(path),
		Data:      data,
		MediaType: mediaType,
		Ext:       ext,
	}
}

func mediaTypeForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".flac":
		return "audio/flac"
	case ".opus":
		return "audio/opus"
	default:
		return ""
	}
}
