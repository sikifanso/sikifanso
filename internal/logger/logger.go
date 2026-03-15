package logger

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alicanalbayrak/sikifanso/internal/paths"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// defaultLogPath returns the path to the sikifanso log file under ~/.sikifanso/clusters/.
func defaultLogPath() (string, error) {
	root, err := paths.RootDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(root, "clusters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating log directory: %w", err)
	}
	return filepath.Join(dir, "sikifanso.log"), nil
}

// New creates a zap logger that writes to both stderr and ~/.sikifanso/clusters/sikifanso.log.
// consoleLevel controls the minimum level for terminal output; the file always logs at DebugLevel.
// Log files are automatically rotated at 10 MB with 3 compressed backups kept for 7 days.
func New(consoleLevel zapcore.Level) (*zap.Logger, func(), error) {
	logPath, err := defaultLogPath()
	if err != nil {
		return nil, nil, err
	}

	lj := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10, // MB
		MaxBackups: 3,
		MaxAge:     7, // days
		Compress:   true,
	}

	consoleEncoderCfg := zap.NewProductionEncoderConfig()
	consoleEncoderCfg.TimeKey = "ts"
	consoleEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

	fileEncoderCfg := zap.NewProductionEncoderConfig()
	fileEncoderCfg.TimeKey = "ts"
	fileEncoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// Console: human-readable for terminal with colored levels
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderCfg)
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stderr), consoleLevel)

	// File: JSON for structured log parsing (no color codes) with rotation
	jsonEncoder := zapcore.NewJSONEncoder(fileEncoderCfg)
	fileCore := zapcore.NewCore(jsonEncoder, zapcore.AddSync(lj), zapcore.DebugLevel)

	// Tee both outputs together
	core := zapcore.NewTee(consoleCore, fileCore)
	logger := zap.New(core)

	cleanup := func() {
		_ = logger.Sync()
		_ = lj.Close()
	}

	return logger, cleanup, nil
}
