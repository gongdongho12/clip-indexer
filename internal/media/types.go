package media

type Config struct {
	Recursive           bool
	Pretty              bool
	IncludeUnsupported  bool
	Trip                string
	FFProbePath         string
	FFMpegPath          string
	Host                string
	Port                int
	UseLLM              bool
	UseLLMVision        bool
	UseLLMAudio         bool
	AutoAnalyze         bool
	AutoAnalyzeMaxItems int
	VisionFrames        int
	VisionMaxItems      int
	AudioMaxSeconds     int
	AudioMaxItems       int
	AudioModel          string
	LLMBaseURL          string
	LLMAPIKey           string
	LLMModel            string
	LLMTimeoutSeconds   int
}

type Report struct {
	Service     ServiceInfo   `json:"service"`
	GeneratedAt string        `json:"generated_at"`
	Options     ReportOptions `json:"options"`
	Items       []Item        `json:"items"`
	Summary     Summary       `json:"summary"`
	Warnings    []string      `json:"warnings,omitempty"`
}

type ServiceInfo struct {
	Name    string `json:"name"`
	CLI     string `json:"cli"`
	Version string `json:"version"`
}

type ReportOptions struct {
	Recursive           bool   `json:"recursive"`
	Trip                string `json:"trip,omitempty"`
	LLM                 bool   `json:"llm"`
	LLMVision           bool   `json:"llm_vision"`
	LLMAudio            bool   `json:"llm_audio"`
	AutoAnalyze         bool   `json:"auto_analyze"`
	AutoAnalyzeMaxItems int    `json:"auto_analyze_max_items,omitempty"`
	VisionFrames        int    `json:"vision_frames,omitempty"`
	VisionMaxItems      int    `json:"vision_max_items,omitempty"`
	AudioMaxSeconds     int    `json:"audio_max_seconds,omitempty"`
	AudioMaxItems       int    `json:"audio_max_items,omitempty"`
}

type Summary struct {
	FilesDiscovered int `json:"files_discovered"`
	FilesIndexed    int `json:"files_indexed"`
	WithShotDate    int `json:"with_shot_date"`
	WithVideoStream int `json:"with_video_stream"`
	WithAudioStream int `json:"with_audio_stream"`
	WithLocation    int `json:"with_location"`
	WithContent     int `json:"with_content"`
	Warnings        int `json:"warnings"`
}

type Item struct {
	SourcePath          string        `json:"source_path"`
	OriginalFileName    string        `json:"original_file_name"`
	Extension           string        `json:"extension"`
	ShotAt              string        `json:"shot_at,omitempty"`
	ShotAtSource        string        `json:"shot_at_source,omitempty"`
	DurationSeconds     float64       `json:"duration_seconds,omitempty"`
	FormatName          string        `json:"format_name,omitempty"`
	Video               *VideoInfo    `json:"video,omitempty"`
	Audio               *AudioInfo    `json:"audio,omitempty"`
	Location            *LocationInfo `json:"location,omitempty"`
	Content             *ContentInfo  `json:"content,omitempty"`
	Group               *GroupInfo    `json:"group,omitempty"`
	Tags                []string      `json:"tags"`
	NameParts           NameParts     `json:"name_parts"`
	RecommendedFileName string        `json:"recommended_file_name"`
	FinalFileName       string        `json:"final_file_name"`
	Confidence          float64       `json:"confidence"`
	LLMNotes            string        `json:"llm_notes,omitempty"`
	Warnings            []string      `json:"warnings,omitempty"`
}

type VideoInfo struct {
	Codec  string  `json:"codec,omitempty"`
	Width  int     `json:"width,omitempty"`
	Height int     `json:"height,omitempty"`
	FPS    float64 `json:"fps,omitempty"`
}

type AudioInfo struct {
	Codec      string `json:"codec,omitempty"`
	Channels   int    `json:"channels,omitempty"`
	SampleRate int    `json:"sample_rate,omitempty"`
}

type LocationInfo struct {
	Latitude       float64 `json:"latitude,omitempty"`
	Longitude      float64 `json:"longitude,omitempty"`
	AltitudeMeters float64 `json:"altitude_meters,omitempty"`
	Label          string  `json:"label,omitempty"`
	Source         string  `json:"source"`
	Confidence     float64 `json:"confidence,omitempty"`
	Notes          string  `json:"notes,omitempty"`
}

type ContentInfo struct {
	SceneSummary       string   `json:"scene_summary,omitempty"`
	AudioSummary       string   `json:"audio_summary,omitempty"`
	AudioTranscript    string   `json:"audio_transcript,omitempty"`
	LocationGuess      string   `json:"location_guess,omitempty"`
	LocationConfidence float64  `json:"location_confidence,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	AudioTags          []string `json:"audio_tags,omitempty"`
	FrameCount         int      `json:"frame_count,omitempty"`
	AudioSeconds       int      `json:"audio_seconds,omitempty"`
	Model              string   `json:"model,omitempty"`
	AudioModel         string   `json:"audio_model,omitempty"`
	Notes              string   `json:"notes,omitempty"`
}

type GroupInfo struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Folder string `json:"folder"`
	Reason string `json:"reason,omitempty"`
}

type NameParts struct {
	Date     string `json:"date,omitempty"`
	Time     string `json:"time,omitempty"`
	Trip     string `json:"trip,omitempty"`
	Slug     string `json:"slug"`
	Sequence string `json:"sequence"`
}

type ProbeResult struct {
	FormatName      string
	DurationSeconds float64
	CreationTimes   []string
	Tags            map[string]string
	Video           *VideoInfo
	Audio           *AudioInfo
	Location        *LocationInfo
	Warnings        []string
}
