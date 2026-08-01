package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	dataRoot      = "/app/rootfs/data"
	filesRoot     = dataRoot + "/data/com.apple.android.music/files"
	dbPath        = filesRoot + "/mpl_db/kvs.sqlitedb"
	fsiPath       = filesRoot + "/fsi.pdat"
	twoFactorPath = filesRoot + "/2fa.txt"
)

type logBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, line := range strings.Split(strings.ReplaceAll(string(p), "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.lines = append(b.lines, time.Now().Format("15:04:05")+"  "+line)
	}
	if len(b.lines) > 500 {
		b.lines = append([]string(nil), b.lines[len(b.lines)-500:]...)
	}
	return len(p), nil
}

func (b *logBuffer) tail(count int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if count <= 0 || count > len(b.lines) {
		count = len(b.lines)
	}
	return append([]string(nil), b.lines[len(b.lines)-count:]...)
}

func (b *logBuffer) mark() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.lines)
}

func (b *logBuffer) containsSince(mark int, value string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if mark < 0 || mark > len(b.lines) {
		mark = 0
	}
	for _, line := range b.lines[mark:] {
		if strings.Contains(line, value) {
			return true
		}
	}
	return false
}

type manager struct {
	mu               sync.Mutex
	cmd              *exec.Cmd
	started          time.Time
	lastExit         string
	mode             string
	twoFactorPending bool
	loginLogMark     int
	logs             logBuffer
	token            string
	wrapperBin       string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type twoFactorRequest struct {
	Code string `json:"code"`
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func prepareRootFS() error {
	dirs := []string{
		dataRoot + "/data/com.apple.android.music/files/mpl_db",
		"/app/rootfs/system/usr/share/zoneinfo",
		"/app/rootfs/etc",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	copies := [][2]string{
		{"/usr/share/zoneinfo/tzdata.zi", "/app/rootfs/system/usr/share/zoneinfo/tzdata"},
		{"/etc/hosts", "/app/rootfs/etc/hosts"},
		{"/etc/resolv.conf", "/app/rootfs/etc/resolv.conf"},
	}
	for _, pair := range copies {
		src, err := os.Open(pair[0])
		if err != nil {
			return err
		}
		dst, err := os.Create(pair[1])
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func (m *manager) runningLocked() bool {
	return m.cmd != nil && m.cmd.Process != nil
}

func (m *manager) start(login *loginRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runningLocked() {
		return errors.New("wrapper is already running")
	}
	args := []string{"-H", "0.0.0.0"}
	if login != nil {
		args = append([]string{"-L", login.Username + ":" + login.Password, "-F"}, args...)
	}
	cmd := exec.Command(m.wrapperBin, args...)
	cmd.Env = append(os.Environ(), "ANDROID_ROOT=/app/rootfs/system", "ANDROID_DATA=/app/rootfs/data")
	cmd.Stdout = &m.logs
	cmd.Stderr = &m.logs
	if err := cmd.Start(); err != nil {
		return err
	}
	m.cmd = cmd
	m.started = time.Now()
	m.lastExit = ""
	mode := "service"
	if login != nil {
		mode = "login"
	}
	m.mode = mode
	m.logs.Write([]byte("Wrapper started in " + mode + " mode\n"))
	go func(active *exec.Cmd) {
		err := active.Wait()
		m.mu.Lock()
		if m.cmd == active {
			m.cmd = nil
			m.mode = ""
			m.twoFactorPending = false
			if err != nil {
				m.lastExit = err.Error()
			} else {
				m.lastExit = "exited normally"
			}
		}
		m.mu.Unlock()
		m.logs.Write([]byte("Wrapper stopped: " + fmt.Sprint(err) + "\n"))
	}(cmd)
	return nil
}

func (m *manager) stop() {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	for i := 0; i < 20; i++ {
		m.mu.Lock()
		done := m.cmd != cmd
		m.mu.Unlock()
		if done {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	for i := 0; i < 20; i++ {
		m.mu.Lock()
		done := m.cmd != cmd
		m.mu.Unlock()
		if done {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *manager) loginState() (running bool, mode string, lastExit string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runningLocked(), m.mode, m.lastExit
}

func validTwoFactorCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (m *manager) waitForLogin(deadline time.Time, detectTwoFactor bool) (string, error) {
	for time.Now().Before(deadline) {
		if fileExists(dbPath) && fileExists(fsiPath) {
			return "complete", nil
		}
		if detectTwoFactor && (m.logs.containsSince(m.loginLogMark, "Waiting for input") || m.logs.containsSince(m.loginLogMark, "Enter your 2FA code")) {
			m.mu.Lock()
			m.twoFactorPending = true
			m.mu.Unlock()
			return "two_factor", nil
		}
		running, mode, lastExit := m.loginState()
		if !running || mode != "login" {
			if lastExit == "" {
				lastExit = "wrapper exited before login completed"
			}
			return "failed", errors.New(lastExit)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return "timeout", errors.New("Apple Music login timed out")
}

func (m *manager) finishLogin(w http.ResponseWriter) {
	m.stop()
	_ = os.Remove(twoFactorPath)
	if err := m.start(nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "login completed but service restart failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "fairplay_registered": true})
}

func (m *manager) authorized(r *http.Request) bool {
	return m.token == "" || r.Header.Get("X-Manager-Token") == m.token
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (m *manager) status(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	running := m.runningLocked()
	started := m.started
	lastExit := m.lastExit
	mode := m.mode
	twoFactorPending := m.twoFactorPending
	m.mu.Unlock()
	uptime := int64(0)
	if running {
		uptime = int64(time.Since(started).Seconds())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"manager":             "ready",
		"wrapper_running":     running,
		"logged_in":           fileExists(dbPath) && fileExists(fsiPath),
		"fairplay_registered": fileExists(fsiPath),
		"login_in_progress":   running && mode == "login",
		"two_factor_required": twoFactorPending,
		"uptime_seconds":      uptime,
		"last_exit":           lastExit,
	})
}

func (m *manager) login(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" || len(req.Password) > 512 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}
	m.stop()
	_ = os.Remove(twoFactorPath)
	m.loginLogMark = m.logs.mark()
	m.mu.Lock()
	m.twoFactorPending = false
	m.mu.Unlock()
	if err := m.start(&req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Do not retain credentials beyond process creation.
	req.Password = ""
	state, err := m.waitForLogin(time.Now().Add(180*time.Second), true)
	switch state {
	case "complete":
		m.finishLogin(w)
	case "two_factor":
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": false, "requires_2fa": true})
	case "timeout":
		m.stop()
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
	default:
		m.stop()
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Apple Music login failed: " + err.Error()})
	}
}

func (m *manager) submitTwoFactor(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req twoFactorRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if !validTwoFactorCode(req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "2FA code must contain exactly six digits"})
		return
	}
	m.mu.Lock()
	running := m.runningLocked()
	loginMode := m.mode == "login"
	pending := m.twoFactorPending
	m.mu.Unlock()
	if !running || !loginMode || !pending {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Apple Music login is not waiting for a 2FA code"})
		return
	}
	if err := os.WriteFile(twoFactorPath, []byte(req.Code), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to submit 2FA code"})
		return
	}
	req.Code = ""
	m.mu.Lock()
	m.twoFactorPending = false
	m.mu.Unlock()
	state, err := m.waitForLogin(time.Now().Add(120*time.Second), false)
	if state == "complete" {
		m.finishLogin(w)
		return
	}
	m.stop()
	if state == "timeout" {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "2FA verification failed: " + err.Error()})
}

func (m *manager) cancelLogin(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	_, mode, _ := m.loginState()
	if mode == "login" {
		m.stop()
	}
	_ = os.Remove(twoFactorPath)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (m *manager) restart(w http.ResponseWriter, r *http.Request) {
	if !m.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if !fileExists(dbPath) || !fileExists(fsiPath) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "login required"})
		return
	}
	m.stop()
	if err := m.start(nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (m *manager) serveLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lines": m.logs.tail(200)})
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://127.0.0.1:18080/healthz")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		return
	}
	if err := prepareRootFS(); err != nil {
		log.Fatalf("prepare rootfs: %v", err)
	}
	wrapperBin := os.Getenv("WRAPPER_BIN")
	if wrapperBin == "" {
		wrapperBin = "/app/wrapper"
	}
	m := &manager{token: os.Getenv("MANAGER_TOKEN"), wrapperBin: wrapperBin}
	if fileExists(dbPath) && fileExists(fsiPath) {
		if err := m.start(nil); err != nil {
			log.Printf("initial wrapper start failed: %v", err)
		}
	} else {
		m.logs.Write([]byte("Login required\n"))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", m.status)
	mux.HandleFunc("GET /status", m.status)
	mux.HandleFunc("GET /logs", m.serveLogs)
	mux.HandleFunc("POST /login", m.login)
	mux.HandleFunc("POST /2fa", m.submitTwoFactor)
	mux.HandleFunc("POST /login/cancel", m.cancelLogin)
	mux.HandleFunc("POST /restart", m.restart)

	server := &http.Server{Addr: ":18080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("wrapper manager listening on :18080")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	m.stop()
	_ = server.Close()
	_ = filepath.Clean(dataRoot)
}
