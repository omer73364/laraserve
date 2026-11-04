// Package metrics samples owned process trees without invoking system commands.
package metrics

import "sync"

type Sample struct {
	Available bool     `json:"available"`
	CPU       *float64 `json:"cpu_percent,omitempty"`
	RSS       uint64   `json:"rss_bytes"`
	Processes int      `json:"processes"`
	Note      string   `json:"note,omitempty"`
}
type reading struct {
	generation           string
	start, ticks, system uint64
}
type Sampler struct {
	mu       sync.Mutex
	previous map[string]reading
}

func New() *Sampler { return &Sampler{previous: map[string]reading{}} }
