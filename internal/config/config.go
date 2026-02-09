package config

import (
	"encoding/json"
	"os"
)

// Config defines optional settings loaded from a JSON file.
type Config struct {
	File          *string `json:"file"`
	Level         *string `json:"level"`
	Since         *string `json:"since"`
	Search        *string `json:"search"`
	JSON          *bool   `json:"json"`
	Limit         *int    `json:"limit"`
	Output        *string `json:"output"`
	Tail          *bool   `json:"tail"`
	TailFromStart *bool   `json:"tailFromStart"`
	TailPoll      *string `json:"tailPoll"`
	Format        *string `json:"format"`
	Store         *string `json:"store"`
	Load          *string `json:"load"`
	Index         *bool   `json:"index"`
	Quiet         *bool   `json:"quiet"`
	StoreHeader   *bool   `json:"storeHeader"`
	Query         *string `json:"query"`
	Explain       *bool   `json:"explain"`
	Replay        *bool   `json:"replay"`
	Snapshot      *string `json:"snapshot"`
	Retention     *string `json:"retention"`
}

// Load reads a JSON config file from disk.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
