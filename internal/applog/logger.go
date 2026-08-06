// Package applog provides global structured logging capabilities.
// By default, only the output is output to the console at startup; after the account is logged in, InitAccountLogger is called to add file output that rotates by day.
// The log file is dropped to <accountDataDir>/logs/YYYY-MM-DD.log.
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// dailyWriter File writer that rotates by day.
// Each time Write checks the current date, automatically closes the old file and opens the new file when the day spans.
type dailyWriter struct {
	mu      sync.Mutex
	dir     string
	curDate string
	file    *os.File
}

func newDailyWriter(dir string) (*dailyWriter, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}
	w := &dailyWriter{dir: dir}
	if err := w.rotate(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *dailyWriter) today() string {
	return time.Now().Format("2006-01-02")
}

func (w *dailyWriter) rotate() error {
	today := w.today()
	if w.file != nil && w.curDate == today {
		return nil
	}
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}
	path := filepath.Join(w.dir, today+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %w", err)
	}
	w.file = f
	w.curDate = today
	return nil
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotate(); err != nil {
		// When rotation fails, downgrade and discard file writing without blocking the main process.
		return len(p), nil
	}
	return w.file.Write(p)
}

func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

// ========== Global log management ==========

var (
	globalMu     sync.RWMutex
	globalLogger *slog.Logger
	fileWriter   *dailyWriter
)

func init() {
	//Default only console output
	globalLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// InitAccountLogger initializes the account-level file log.
// After the call, the log is output to the console and the daily file under <accountDataDir>/logs/ at the same time.
// Repeated calls will close the old file writer first.
func InitAccountLogger(accountDataDir string) error {
	logsDir := filepath.Join(accountDataDir, "logs")
	dw, err := newDailyWriter(logsDir)
	if err != nil {
		return err
	}

	globalMu.Lock()
	defer globalMu.Unlock()

	// Close the old file writer
	if fileWriter != nil {
		fileWriter.Close()
	}
	fileWriter = dw

	//Multiple output: console + file
	multi := io.MultiWriter(os.Stderr, dw)
	globalLogger = slog.New(slog.NewTextHandler(multi, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return nil
}

// CloseAccountLogger closes the account-level file log and returns to pure console output.
func CloseAccountLogger() {
	globalMu.Lock()
	defer globalMu.Unlock()

	if fileWriter != nil {
		fileWriter.Close()
		fileWriter = nil
	}
	globalLogger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// Logger returns the current global Logger
func Logger() *slog.Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// ========== Convenience methods ==========

// Debug output debug log
func Debug(msg string, args ...any) {
	Logger().Debug(msg, args...)
}

// Info output information log
func Info(msg string, args ...any) {
	Logger().Info(msg, args...)
}

// Warn output warning log
func Warn(msg string, args ...any) {
	Logger().Warn(msg, args...)
}

// Error output error log
func Error(msg string, args ...any) {
	Logger().Error(msg, args...)
}
