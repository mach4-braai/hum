package protocol

type SessionPayload struct {
	ID        string            `json:"id"`
	Workspace string            `json:"workspace,omitempty"`
	Title     string            `json:"title,omitempty"`
	State     string            `json:"state"`
	Pitch     string            `json:"pitch,omitempty"`
	Priority  int               `json:"priority,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Updates   int               `json:"updates"`
	Seconds   float64           `json:"seconds"`
}

type StatusPayload struct {
	Sessions       []SessionPayload `json:"sessions"`
	Theme          string           `json:"theme"`
	Root           string           `json:"root"`
	Scale          string           `json:"scale"`
	ContextOwner   string           `json:"context_owner,omitempty"`
	Renderer       string           `json:"renderer"`
	SampleRate     int              `json:"sample_rate"`
	Version        string           `json:"version"`
	Volume         float64          `json:"volume"`
	Muted          bool             `json:"muted"`
	SoundingVoices int              `json:"sounding_voices"`
}

type ThemeListPayload struct {
	Themes []string `json:"themes"`
	Active string   `json:"active"`
}

type ThemeUsePayload struct {
	Theme string `json:"theme"`
}

type AudioTestPayload struct {
	Played   bool    `json:"played"`
	Renderer string  `json:"renderer"`
	Muted    bool    `json:"muted"`
	Seconds  float64 `json:"seconds"`
}
