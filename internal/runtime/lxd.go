package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type LXD struct {
	Binary      string
	StateDir    string
	ControlPort int
	Process     *exec.Cmd
	Client      *http.Client
	mu          sync.Mutex
	done        chan error
}

func NewLXD(binary, stateDir string) *LXD {
	return &LXD{Binary: binary, StateDir: stateDir}
}

func FreeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (l *LXD) Start(ctx context.Context, seed []byte) error {
	l.mu.Lock()
	running := l.Process != nil && l.Process.Process != nil
	l.mu.Unlock()
	if running {
		return nil
	}
	if l.Binary == "" {
		return errors.New("sing-box-lx binary is empty")
	}
	if _, err := os.Stat(l.Binary); err != nil {
		return fmt.Errorf("sing-box-lx: %w", err)
	}
	if err := os.MkdirAll(l.StateDir, 0755); err != nil {
		return err
	}
	if l.ControlPort == 0 {
		p, err := FreeTCPPort()
		if err != nil {
			return err
		}
		l.ControlPort = p
	}
	daemon := map[string]any{
		"listen": fmt.Sprintf("127.0.0.1:%d", l.ControlPort),
		"tls":    false,
		"secret": "",
	}
	daemonBytes, _ := json.Marshal(daemon)
	if err := os.WriteFile(filepath.Join(l.StateDir, "daemon.json"), daemonBytes, 0600); err != nil {
		return err
	}
	seedPath := filepath.Join(l.StateDir, "seed.json")
	if len(seed) > 0 {
		if err := os.WriteFile(seedPath, seed, 0600); err != nil {
			return err
		}
	}
	args := []string{"lxd", "--state-dir", l.StateDir}
	if len(seed) > 0 {
		args = append(args, "-c", seedPath)
	}
	cmd := exec.Command(l.Binary, args...)
	cmd.Dir = l.StateDir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start sing-box-lx lxd: %w", err)
	}
	l.Process = cmd
	l.done = make(chan error, 1)
	l.Client = &http.Client{Timeout: 8 * time.Second}
	done := l.done
	go func() {
		err := cmd.Wait()
		done <- err
		close(done)
		l.mu.Lock()
		if l.Process == cmd {
			l.Process = nil
		}
		l.mu.Unlock()
	}()
	if err := l.waitReady(ctx); err != nil {
		_ = l.Stop()
		return err
	}
	return nil
}

func (l *LXD) endpoint(path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", l.ControlPort, path)
}

func (l *LXD) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, l.endpoint("/admin/status"), nil)
		resp, err := l.Client.Do(req)
		if err == nil {
			var status struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&status)
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("lxd control channel startup timeout")
		case <-ticker.C:
		}
	}
}

func (l *LXD) Apply(ctx context.Context, config []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint("/admin/apply"), bytes.NewReader(config))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := ioReadAllLimit(resp.Body, 2<<20)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lxd apply failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func ioReadAllLimit(r io.Reader, n int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, n))
}

func (l *LXD) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.Process == nil || l.Process.Process == nil {
		return nil
	}
	proc := l.Process
	_ = proc.Process.Kill()
	if l.done != nil {
		select {
		case <-l.done:
		case <-time.After(3 * time.Second):
		}
	}
	l.Process = nil
	l.done = nil
	return nil
}

func (l *LXD) Restart(ctx context.Context, config []byte) error {
	if err := l.Stop(); err != nil {
		return err
	}
	if err := l.Start(ctx, config); err != nil {
		return err
	}
	return l.Apply(ctx, config)
}
