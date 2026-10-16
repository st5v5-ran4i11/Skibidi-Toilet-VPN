package checker

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/naming"
	NodeRanker "github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/ranking"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/processor"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"
)

const modePoolLimit = 1500

// PipelineConsumer сохраняет только общие пулы.
//
// Генерируются ровно эти файлы:
//
//	all.txt / all_home.txt / all_mobile.txt
//	current.txt / current_home.txt / current_mobile.txt
//	stable.txt / stable_home.txt / stable_mobile.txt
//	tv.txt (up to 100 top-ranked nodes from this run)
//
// Отдельных youtube/telegram/global/universal подписок больше нет.
// Результаты Deep Check по сервисам используются только для общего Score.
func PipelineConsumer(ctx context.Context, in <-chan model.FinalResult, outputDir, mode string, limit, stableLimit int, historyDir string, pm *progress.Manager) ([]model.FinalResult, error) {
	if pm != nil {
		pm.AddStage("RANKING", progress.UnknownTotal, 1)
		pm.StartStage("RANKING")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return nil, err
	}
	mode = normalizeMode(mode)
	if limit <= 0 || limit > modePoolLimit {
		limit = modePoolLimit
	}
	if stableLimit <= 0 || stableLimit > modePoolLimit {
		stableLimit = modePoolLimit
	}
	dedup := processor.NewConcurrentDedup()
	historyPath := filepath.Join(historyDir, mode+".json")
	history, err := loadNodeHistory(historyPath)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	current := make([]model.FinalResult, 0, 2048)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result, ok := <-in:
			if !ok {
				goto done
			}
			naming.Generate(&result.Node)
			updateNodeHistory(history, result, now, dedup)
			if result.Alive && dedup.Fingerprint(result.Node.Raw) != "" {
				current = append(current, result)
				if pm != nil {
					pm.Success("RANKING")
					pm.Produce("RANKING", 1)
				}
			} else if pm != nil {
				pm.Fail("RANKING")
			}
		}
	}
done:
	if pm != nil {
		pm.Complete("RANKING")
		pm.AddStage("WRITING", progress.UnknownTotal, 1)
		pm.StartStage("WRITING")
	}
	pruneNodeHistory(history, now)
	if err := saveNodeHistory(historyPath, history); err != nil {
		return nil, err
	}
	NodeRanker.NodeRanker{}.Rank(current)
	if err := writeTVPool(outputDir, current); err != nil {
		return nil, err
	}
	if err := writeResultPool(outputDir, "current_"+mode+".txt", current, limit, "CURRENT", false, dedup); err != nil {
		return nil, err
	}
	// ALL is cumulative: merge the previous pool with successful results from this run.
	previous, err := readPool(filepath.Join(outputDir, "all_"+mode+".txt"))
	if err != nil {
		return nil, err
	}
	cumulative := append(previous, resultsToNodes(current)...)
	cumulative = dedupNodeInfos(cumulative)
	ranked := rankNodesByHistory(cumulative, history)
	if len(ranked) > modePoolLimit {
		ranked = ranked[:modePoolLimit]
	}
	if err := writeNodePool(outputDir, "all_"+mode+".txt", ranked, modePoolLimit, "ALL-"+strings.ToUpper(mode)); err != nil {
		return nil, err
	}
	stable := stableHistoryEntries(history)
	if err := writeHistoryPool(outputDir, "stable_"+mode+".txt", stable, stableLimit, "STABLE-"+strings.ToUpper(mode), dedup); err != nil {
		return nil, err
	}
	if err := rebuildUnionFiles(outputDir, dedup); err != nil {
		if pm != nil {
			pm.FailStage("WRITING")
		}
		return nil, err
	}
	if pm != nil {
		pm.SetTotal("WRITING", len(current))
		pm.Add("WRITING", len(current))
		pm.AddSuccess("WRITING", len(current))
		pm.Produce("WRITING", len(current))
		pm.Complete("WRITING")
	}
	return current, nil
}

func resultsToNodes(in []model.FinalResult) []model.NodeInfo {
	out := make([]model.NodeInfo, 0, len(in))
	for _, r := range in {
		out = append(out, r.Node)
	}
	return out
}
func readPool(path string) ([]model.NodeInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []model.NodeInfo
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n, err := ParseNode(stripRemark(line))
		if err == nil {
			out = append(out, n)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
func stripRemark(s string) string {
	if i := strings.Index(s, "#"); i >= 0 {
		return s[:i]
	}
	return s
}
func dedupNodeInfos(in []model.NodeInfo) []model.NodeInfo {
	d := processor.NewConcurrentDedup()
	out := make([]model.NodeInfo, 0, len(in))
	for _, n := range in {
		if d.IsUnique(n.Raw) {
			out = append(out, n)
		}
	}
	return out
}
func filterCumulative(nodes []model.NodeInfo, h *NodeHistory) []model.NodeInfo {
	if h == nil {
		return nodes
	}
	d := processor.NewConcurrentDedup()
	out := make([]model.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		fp := d.Fingerprint(n.Raw)
		if fp == "" {
			continue
		}
		entry, ok := h.Nodes[fp]
		if ok && entry.ConsecutiveFailures >= stableFailureTolerance {
			continue
		}
		out = append(out, n)
	}
	return out
}

func rankNodesByHistory(nodes []model.NodeInfo, h *NodeHistory) []model.NodeInfo {
	sort.SliceStable(nodes, func(i, j int) bool {
		a := h.Nodes[processor.NewConcurrentDedup().Fingerprint(nodes[i].Raw)]
		b := h.Nodes[processor.NewConcurrentDedup().Fingerprint(nodes[j].Raw)]
		if a.LastScore != b.LastScore {
			return a.LastScore > b.LastScore
		}
		if a.Successes != b.Successes {
			return a.Successes > b.Successes
		}
		return a.LastLatency < b.LastLatency
	})
	return nodes
}
func writeNodePool(dir, file string, nodes []model.NodeInfo, limit int, label string) error {
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}
	lines := make([]string, 0, len(nodes))
	for i, n := range nodes {
		if n.DisplayName == "" {
			n.DisplayName = n.Remark
		}
		raw := stripRemark(n.Raw)
		if raw != "" {
			lines = append(lines, raw+"#"+escapeRemark(label+"-"+itoa(i+1)))
		}
	}
	return writeLines(dir, file, lines)
}

func normalizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "mobile") {
		return "mobile"
	}
	return "home"
}

func sortFinalResults(results []model.FinalResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Latency != results[j].Latency {
			return results[i].Latency < results[j].Latency
		}
		if results[i].Node.TCPDelay != results[j].Node.TCPDelay {
			return results[i].Node.TCPDelay < results[j].Node.TCPDelay
		}
		return results[i].Node.Host+":"+results[i].Node.Port < results[j].Node.Host+":"+results[j].Node.Port
	})
}

// writeTVPool атомарно публикует до 100 лучших узлов, прошедших Deep Check.
// Неуспешные проверки исключаются; дедупликация выполняется по fingerprint узла.
func writeTVPool(outputDir string, ranked []model.FinalResult) error {
	seen := processor.NewConcurrentDedup()
	lines := make([]string, 0, 100)
	for _, result := range ranked {
		if !result.Alive {
			continue
		}
		if strings.TrimSpace(result.Node.Raw) == "" || !seen.IsUnique(result.Node.Raw) {
			continue
		}
		lines = append(lines, cleanPoolLine(result.Node.Raw, result.Node.DisplayName, "TV", len(lines), 1, false))
		if len(lines) == 100 {
			break
		}
	}
	return writeLinesAtomic(outputDir, "tv.txt", lines)
}

func writeLinesAtomic(outputDir, filename string, lines []string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	var b strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	tmp, err := os.CreateTemp(outputDir, "."+filename+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(outputDir, filename))
}

func writeResultPool(
	outputDir string,
	filename string,
	results []model.FinalResult,
	limit int,
	label string,
	stable bool,
	dedup *processor.ConcurrentDedup,
) error {
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	lines := make([]string, 0, len(results))
	for i, result := range results {
		counter := 1
		if stable {
			counter = result.StabilityScore
			if counter < 1 {
				counter = 1
			}
		}
		lines = append(lines, cleanPoolLine(result.Node.Raw, result.Node.DisplayName, label, i, counter, stable))
	}

	return writeLines(outputDir, filename, lines)
}

func writeHistoryPool(
	outputDir string,
	filename string,
	entries []HistoryEntry,
	limit int,
	label string,
	dedup *processor.ConcurrentDedup,
) error {
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}

	lines := make([]string, 0, len(entries))
	for i, entry := range entries {
		if dedup.Fingerprint(entry.Raw) == "" {
			continue
		}
		displayName := ""
		if node, err := ParseNode(entry.Raw); err == nil {
			naming.Generate(&node)
			displayName = node.DisplayName
		}
		lines = append(lines, cleanPoolLine(entry.Raw, displayName, label, i, entry.Successes, true))
	}

	return writeLines(outputDir, filename, lines)
}

func cleanPoolLine(raw, displayName, label string, index, counter int, stable bool) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "#"); idx >= 0 {
		raw = raw[:idx]
	}
	if raw == "" {
		return ""
	}

	remark := strings.TrimSpace(displayName)
	if remark == "" {
		remark = label + "-" + itoa(index+1)
	}
	if stable {
		remark += "-OK-" + itoa(counter)
	}
	return raw + "#" + escapeRemark(remark)
}

func escapeRemark(value string) string {
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "#", "")
	return value
}

func itoa(value int) string {
	// Avoid pulling strconv into every call site in this small writer.
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := make([]byte, 0, 12)
	for value > 0 {
		buf = append(buf, byte('0'+value%10))
		value /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	if negative {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func writeLines(outputDir, filename string, lines []string) error {
	var builder strings.Builder
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}

	return os.WriteFile(filepath.Join(outputDir, filename), []byte(builder.String()), 0644)
}

func rebuildUnionFiles(outputDir string, dedup *processor.ConcurrentDedup) error {
	all, err := mergeFiles(outputDir, dedup,
		"all_home.txt",
		"all_mobile.txt",
	)
	if err != nil {
		return err
	}
	if err := writeLines(outputDir, "all.txt", all); err != nil {
		return err
	}

	current, err := mergeFiles(outputDir, dedup,
		"current_home.txt",
		"current_mobile.txt",
	)
	if err != nil {
		return err
	}
	if err := writeLines(outputDir, "current.txt", current); err != nil {
		return err
	}

	stable, err := mergeFiles(outputDir, dedup,
		"stable_home.txt",
		"stable_mobile.txt",
	)
	if err != nil {
		return err
	}
	return writeLines(outputDir, "stable.txt", stable)
}

func mergeFiles(outputDir string, dedup *processor.ConcurrentDedup, filenames ...string) ([]string, error) {
	dedup.Reset()
	lines := make([]string, 0, 3000)

	for _, filename := range filenames {
		file, err := os.Open(filepath.Join(outputDir, filename))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if dedup.IsUnique(line) {
				lines = append(lines, line)
			}
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			if logger.Log != nil {
				logger.Log.Error("ошибка чтения пула "+filename, zap.Error(err))
			}
			return nil, err
		}
		_ = file.Close()
	}

	return lines, nil
}
