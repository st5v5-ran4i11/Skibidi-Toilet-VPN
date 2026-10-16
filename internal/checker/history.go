package checker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/naming"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/processor"
)

const (
	stableSuccessThreshold = 3
	stableFailureTolerance = 3
	historyRetention       = 60 * 24 * time.Hour
)

type HistoryEntry struct {
	Raw                 string             `json:"raw"`
	FirstSeen           time.Time          `json:"first_seen"`
	LastSeen            time.Time          `json:"last_seen"`
	LastSuccess         time.Time          `json:"last_success,omitempty"`
	Checks              int                `json:"checks"`
	Successes           int                `json:"successes"`
	Failures            int                `json:"failures"`
	ConsecutiveFailures int                `json:"consecutive_failures"`
	LastLatency         time.Duration      `json:"last_latency"`
	LastScore           int                `json:"last_score"`
	ServiceScores       map[string]float64 `json:"service_scores,omitempty"`
	ServiceSuccesses    map[string]int     `json:"service_successes,omitempty"`
	ServiceFailures     map[string]int     `json:"service_failures,omitempty"`
	SuccessServices     map[string]bool    `json:"success_services,omitempty"`
}

type NodeHistory struct {
	Version int                     `json:"version"`
	Nodes   map[string]HistoryEntry `json:"nodes"`
}

func newNodeHistory() *NodeHistory {
	return &NodeHistory{
		Version: 1,
		Nodes:   make(map[string]HistoryEntry),
	}
}

func loadNodeHistory(path string) (*NodeHistory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newNodeHistory(), nil
		}
		return nil, err
	}

	var history NodeHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	if history.Nodes == nil {
		history.Nodes = make(map[string]HistoryEntry)
	}
	if history.Version == 0 {
		history.Version = 1
	}
	return &history, nil
}

func saveNodeHistory(path string, history *NodeHistory) error {
	if history == nil {
		return nil
	}
	if history.Nodes == nil {
		history.Nodes = make(map[string]HistoryEntry)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	// Windows не заменяет существующий файл через Rename, поэтому сначала
	// удаляем старую версию. При сбое старый файл всё равно уже не портится.
	_ = os.Remove(path)
	return os.Rename(tmp, path)
}

func updateNodeHistory(history *NodeHistory, result model.FinalResult, now time.Time, dedup *processor.ConcurrentDedup) string {
	if history == nil {
		return ""
	}
	if history.Nodes == nil {
		history.Nodes = make(map[string]HistoryEntry)
	}

	fp := dedup.Fingerprint(result.Node.Raw)
	if fp == "" {
		return ""
	}

	entry, exists := history.Nodes[fp]
	if !exists {
		entry = HistoryEntry{
			Raw:             result.Node.Raw,
			FirstSeen:       now,
			SuccessServices: make(map[string]bool),
		}
	}

	naming.Generate(&result.Node)
	entry.Raw = result.Node.Raw
	entry.LastSeen = now
	entry.Checks++

	if result.Alive {
		entry.LastLatency = result.Latency
		entry.LastScore = result.Score
		entry.Successes++
		entry.ConsecutiveFailures = 0
		entry.LastSuccess = now
		entry.SuccessServices = copyBoolMap(result.SuccessServices)
	} else {
		entry.Failures++
		entry.ConsecutiveFailures++
	}

	history.Nodes[fp] = entry
	return fp
}

func pruneNodeHistory(history *NodeHistory, now time.Time) {
	if history == nil {
		return
	}
	cutoff := now.Add(-historyRetention)
	for fp, entry := range history.Nodes {
		if entry.LastSeen.Before(cutoff) {
			delete(history.Nodes, fp)
		}
	}
}

func stableHistoryEntries(history *NodeHistory) []HistoryEntry {
	if history == nil {
		return nil
	}

	entries := make([]HistoryEntry, 0, len(history.Nodes))
	for _, entry := range history.Nodes {
		if entry.Raw == "" || entry.Successes < stableSuccessThreshold || entry.ConsecutiveFailures >= stableFailureTolerance {
			continue
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].LastScore != entries[j].LastScore {
			return entries[i].LastScore > entries[j].LastScore
		}
		if entries[i].Successes != entries[j].Successes {
			return entries[i].Successes > entries[j].Successes
		}
		if entries[i].LastLatency != entries[j].LastLatency {
			return entries[i].LastLatency < entries[j].LastLatency
		}
		return entries[i].LastSuccess.After(entries[j].LastSuccess)
	})

	return entries
}

func copyBoolMap(src map[string]bool) map[string]bool {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]bool, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
