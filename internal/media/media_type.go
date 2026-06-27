package media

import (
	"path/filepath"
	"strings"
)

const (
	mediaTypeVideo = "video"
	mediaTypeImage = "image"
	mediaTypeAudio = "audio"
	mediaTypeOther = "other"
)

var imageExtensions = map[string]bool{
	".avif": true,
	".heic": true,
	".heif": true,
	".jpeg": true,
	".jpg":  true,
	".png":  true,
	".tif":  true,
	".tiff": true,
	".webp": true,
}

var audioExtensions = map[string]bool{
	".aac":  true,
	".aiff": true,
	".alac": true,
	".flac": true,
	".m4a":  true,
	".mp3":  true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".wma":  true,
}

func supportedMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case videoExtensions[ext]:
		return mediaTypeVideo
	case imageExtensions[ext]:
		return mediaTypeImage
	case audioExtensions[ext]:
		return mediaTypeAudio
	default:
		return ""
	}
}

func mediaTypeForPath(path string, probe ProbeResult) string {
	if mediaType := supportedMediaType(path); mediaType != "" {
		return mediaType
	}
	switch {
	case probe.Video != nil:
		return mediaTypeVideo
	case probe.Audio != nil:
		return mediaTypeAudio
	default:
		return mediaTypeOther
	}
}

func itemMediaType(item Item) string {
	if strings.TrimSpace(item.MediaType) != "" {
		return item.MediaType
	}
	return mediaTypeForPath(item.SourcePath, ProbeResult{Video: item.Video, Audio: item.Audio})
}
