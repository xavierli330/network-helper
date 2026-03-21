package model

import "time"

type Snapshot struct {
	ID         int       `json:"id"`
	DeviceID   string    `json:"device_id"`
	CapturedAt time.Time `json:"captured_at"`
	SourceFile string    `json:"source_file"`
	Commands   string    `json:"commands"`
}

type ConfigSnapshot struct {
	ID           int       `json:"id"`
	DeviceID     string    `json:"device_id"`
	ConfigText   string    `json:"config_text"`
	DiffFromPrev string    `json:"diff_from_prev"`
	CapturedAt   time.Time `json:"captured_at"`
	SourceFile   string    `json:"source_file"`
}

type TroubleshootLog struct {
	ID           int       `json:"id"`
	DeviceID     string    `json:"device_id"`
	Symptom      string    `json:"symptom"`
	CommandsUsed string    `json:"commands_used"`
	Findings     string    `json:"findings"`
	Resolution   string    `json:"resolution"`
	Tags         string    `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
}

type CommandReference struct {
	ID            int    `json:"id"`
	Vendor        string `json:"vendor"`
	Command       string `json:"command"`
	Description   string `json:"description"`
	ExampleOutput string `json:"example_output"`
	ParseHint     string `json:"parse_hint"`
	SourceURL     string `json:"source_url"`
}

type LogIngestion struct {
	ID          int       `json:"id"`
	FilePath    string    `json:"file_path"`
	FileHash    string    `json:"file_hash"`
	LastOffset  int64     `json:"last_offset"`
	ProcessedAt time.Time `json:"processed_at"`
}
