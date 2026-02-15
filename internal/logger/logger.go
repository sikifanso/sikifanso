package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a zap logger that writes to both stderr and the given file path.
func New(filePath string) (*zap.Logger, func(), error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// Console: human-readable for terminal
	consoleEncoder := zapcore.NewConsoleEncoder(encoderCfg)
	consoleCore := zapcore.NewCore(consoleEncoder, zapcore.Lock(os.Stderr), zapcore.DebugLevel)

	// File: JSON for structured log parsing
	jsonEncoder := zapcore.NewJSONEncoder(encoderCfg)
	fileCore := zapcore.NewCore(jsonEncoder, zapcore.AddSync(file), zapcore.DebugLevel)

	// Tee both outputs together
	core := zapcore.NewTee(consoleCore, fileCore)
	logger := zap.New(core)

	cleanup := func() {
		_ = logger.Sync()
		_ = file.Close()
	}

	return logger, cleanup, nil
}
