package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a zap logger that writes to both stderr and the given file path.
// consoleLevel controls the minimum level for terminal output; the file always logs at DebugLevel.
func New(filePath string, consoleLevel zapcore.Level) (*zap.Logger, func(), error) {
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
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

	// File: JSON for structured log parsing (no color codes)
	jsonEncoder := zapcore.NewJSONEncoder(fileEncoderCfg)
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
