package config

import (
	"encoding/json"
	"fmt"
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
	SnapshotLoad  *string `json:"snapshotLoad"`
	Retention     *string `json:"retention"`
	Metrics       *bool   `json:"metrics"`
	MetricsFile   *string `json:"metricsFile"`
	Serve         *bool   `json:"serve"`
	Port          *int    `json:"port"`
	ShardDir      *string `json:"shardDir"`
	ShardRead     *bool   `json:"shardRead"`
	ApiKey        *string `json:"apiKey"`
}

// Load reads a JSON config file from disk.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found: %s (create it or use a different --config path)", path)
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
