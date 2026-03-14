package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// New creates a zap logger that writes to both stderr and the given file path.
// consoleLevel controls the minimum level for terminal output; the file always logs at DebugLevel.
// Log files are automatically rotated at 10 MB with 3 compressed backups kept for 7 days.
func New(filePath string, consoleLevel zapcore.Level) (*zap.Logger, func(), error) {
	lj := &lumberjack.Logger{
		Filename:   filePath,
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
