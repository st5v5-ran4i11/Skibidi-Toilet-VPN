package fetcher

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/control"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/processor"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"

	"go.uber.org/zap"
)

type Fetcher struct {
	cfg     *config.Config
	sources *config.SourcesConfig
	pm      *progress.Manager
	gate    control.Gate
	client  *http.Client
}

func New(
	cfg *config.Config,
	sources *config.SourcesConfig,
	pm *progress.Manager,
	gate ...control.Gate,
) *Fetcher {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 0,
	}

	var g control.Gate
	if len(gate) > 0 {
		g = gate[0]
	}

	return &Fetcher{
		cfg:     cfg,
		sources: sources,
		pm:      pm,
		gate:    g,
		client: &http.Client{
			Transport: transport,
			Timeout:   0, // Таймаут контролируется контекстами индивидуально
		},
	}
}

func (f *Fetcher) StreamFetch(
	ctx context.Context,
	externalDedup *processor.ConcurrentDedup,
) <-chan string {
	out := make(chan string, 1000)

	if len(f.sources.Sources) == 0 {
		close(out)
		return out
	}

	dedup := externalDedup
	if dedup == nil {
		dedup = processor.NewConcurrentDedup()
	}

	activeProfile := f.cfg.GetActiveProfile()
	workers := activeProfile.FetcherWorkers
	if workers <= 0 {
		workers = 10
	}

	if logger.Log != nil {
		logger.Log.Info(
			"Fetcher запущен",
			zap.String("profile", f.cfg.NetworkMode),
			zap.Int("workers", workers),
		)
	}

	seenURLs := make(map[string]struct{})
	var sources []config.Source

	for _, src := range f.sources.Sources {
		if !src.Enabled {
			continue
		}

		urlStr := strings.TrimSpace(src.URL)
		if urlStr == "" {
			continue
		}

		if _, exists := seenURLs[urlStr]; exists {
			continue
		}
		seenURLs[urlStr] = struct{}{}

		src.URL = urlStr
		sources = append(sources, src)
	}

	if logger.Log != nil && len(sources) != len(f.sources.Sources) {
		logger.Log.Info(
			"Дубликаты источников пропущены",
			zap.Int("configured_sources", len(f.sources.Sources)),
			zap.Int("unique_sources", len(sources)),
			zap.Int("duplicates_skipped", len(f.sources.Sources)-len(sources)),
		)
	}

	// Total должен отражать фактическое число уникальных URL,
	// которые будут поставлены в очередь. В sources.yaml могут встречаться
	// одинаковые URL под разными именами — второй раз такой источник
	// скачивать бессмысленно.
	if f.pm != nil {
		f.pm.SetTotal("FETCHER", len(sources))
		f.pm.StartStage("FETCHER")
	}

	jobs := make(chan config.Source)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case src, ok := <-jobs:
					if !ok {
						return
					}
					if f.gate != nil && f.gate.Wait(ctx) != nil {
						return
					}
					f.fetchSourceWithTimeout(ctx, src, out, dedup)
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, src := range sources {
			select {
			case <-ctx.Done():
				return
			case jobs <- src:
			}
		}
	}()

	go func() {
		wg.Wait()
		if f.pm != nil {
			f.pm.Complete("FETCHER")
		}
		close(out)
	}()

	return out
}

func (f *Fetcher) fetchSourceWithTimeout(
	parentCtx context.Context,
	src config.Source,
	out chan<- string,
	dedup *processor.ConcurrentDedup,
) {
	startTime := time.Now()
	urlStr := strings.TrimSpace(src.URL)
	if urlStr == "" {
		if f.pm != nil {
			f.pm.Fail("FETCHER")
		}
		return
	}

	urlStr = strings.ReplaceAll(urlStr, "githubusercintent.com", "githubusercontent.com")

	fetchTimeout := time.Duration(f.cfg.Fetcher.Timeout) * time.Second
	if fetchTimeout <= 0 {
		fetchTimeout = 30 * time.Second
	}

	const maxRetries = 3
	var bodyBytes []byte
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if parentCtx.Err() != nil {
			return
		}

		fetchCtx, cancel := context.WithTimeout(parentCtx, fetchTimeout)
		err := func() error {
			defer cancel()
			finalURL := addCacheBuster(urlStr)

			req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, finalURL, nil)
			if err != nil {
				return err
			}

			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("Cache-Control", "no-cache")
			req.Header.Set("Accept", "*/*")

			resp, err := f.client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("HTTP status %d", resp.StatusCode)
			}

			// Ограничиваем чтение 50 МБ на файл для защиты от Zip-бомб
			limitedReader := io.LimitReader(resp.Body, 50*1024*1024)
			data, err := io.ReadAll(limitedReader)
			if err != nil {
				return err
			}

			bodyBytes = data
			return nil
		}()

		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err

		if logger.Log != nil {
			logger.Log.Warn(
				"Ошибка скачивания источника",
				zap.String("source_name", src.Name),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
		}

		if attempt < maxRetries {
			select {
			case <-parentCtx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	if lastErr != nil {
		if f.pm != nil {
			f.pm.Fail("FETCHER")
		}
		if logger.Log != nil {
			logger.Log.Warn(
				"Источник окончательно недоступен",
				zap.String("source_name", src.Name),
				zap.String("url", urlStr),
				zap.Error(lastErr),
			)
		}
		return
	}

	if len(bodyBytes) == 0 {
		if f.pm != nil {
			f.pm.Fail("FETCHER")
		}
		return
	}

	bodyStr := decodeSubscription(string(bodyBytes))
	bodyStr = strings.ReplaceAll(bodyStr, "\r", "")
	lines := strings.Split(bodyStr, "\n")

	nodesCount := 0
	uniqueNodesSent := 0

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		nodesCount++
		if !dedup.IsUnique(line) {
			continue
		}

		select {
		case <-parentCtx.Done():
			return
		case out <- line:
			uniqueNodesSent++
		}
	}

	if f.pm != nil {
		f.pm.Success("FETCHER")
		if uniqueNodesSent > 0 {
			// ИСПРАВЛЕНО: передаем точное количество новых строк в GUI менеджер
			f.pm.Produce("FETCHER", uniqueNodesSent)
		}
	}

	if logger.Log != nil {
		logger.Log.Info(
			"Источник успешно обработан",
			zap.String("source_name", src.Name),
			zap.String("url", urlStr),
			zap.Bool("available", true),
			zap.Int("nodes_downloaded", nodesCount),
			zap.Int("unique_nodes_added", uniqueNodesSent),
			zap.Duration("duration", time.Since(startTime)),
		)
	}
}

func addCacheBuster(urlStr string) string {
	cacheBuster := fmt.Sprintf("nocache=%d", time.Now().UnixNano())
	if strings.Contains(urlStr, "?") {
		return urlStr + "&" + cacheBuster
	}
	return urlStr + "?" + cacheBuster
}

func decodeSubscription(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	// Если в строке уже содержатся готовые протоколы, декодирование не требуется
	if strings.Contains(trimmed, "://") {
		return trimmed
	}

	// Тотальная очистка строки для работы с любыми вариациями Base64 подписок
	cleaned := trimmed
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, "\n", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "\t", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "+")
	cleaned = strings.ReplaceAll(cleaned, "_", "/")

	// Нормализация длины (добавление недостающих знаков равенства)
	switch len(cleaned) % 4 {
	case 2:
		cleaned += "=="
	case 3:
		cleaned += "="
	}

	// ИСПРАВЛЕНО: Безопасное пошаговое декодирование через стандартные и сырые таблицы алфавита
	decodedBytes, err := base64.StdEncoding.DecodeString(cleaned)
	if err != nil {
		decodedBytes, err = base64.RawStdEncoding.DecodeString(cleaned)
	}
	if err != nil {
		// Если декодировать не удалось — отдаем исходный текст (возможно, это кастомный формат или plain-text)
		return trimmed
	}

	decoded := strings.TrimSpace(string(decodedBytes))
	if decoded == "" {
		return trimmed
	}

	return decoded
}

func normalizeSourceLine(line, protocol string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false
	}
	if strings.Contains(line, "://") {
		return line, true
	}
	p := strings.ToLower(strings.TrimSpace(protocol))
	if p == "auto" {
		return line, true
	}
	if !strings.Contains(line, ":") {
		return "", false
	}
	switch p {
	case "http", "https", "socks4", "socks4a", "socks5", "socks":
		return p + "://" + line, true
	default:
		return line, true
	}
}
