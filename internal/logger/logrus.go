package logger

import (
	"io"

	k3dlog "github.com/k3d-io/k3d/v5/pkg/logger"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// zapHook is a logrus.Hook that forwards entries to a zap.Logger.
type zapHook struct {
	zap *zap.Logger
}

func (h *zapHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *zapHook) Fire(entry *logrus.Entry) error {
	fields := make([]zap.Field, 0, len(entry.Data))
	for k, v := range entry.Data {
		fields = append(fields, zap.Any(k, v))
	}

	lvl := logrusToZap(entry.Level)
	if ce := h.zap.Check(lvl, entry.Message); ce != nil {
		ce.Write(fields...)
	}
	return nil
}

func logrusToZap(lvl logrus.Level) zapcore.Level {
	switch lvl {
	case logrus.TraceLevel, logrus.DebugLevel:
		return zapcore.DebugLevel
	case logrus.InfoLevel:
		return zapcore.InfoLevel
	case logrus.WarnLevel:
		return zapcore.WarnLevel
	case logrus.ErrorLevel:
		return zapcore.ErrorLevel
	case logrus.FatalLevel:
		return zapcore.FatalLevel
	case logrus.PanicLevel:
		return zapcore.DPanicLevel
	default:
		return zapcore.InfoLevel
	}
}

// RedirectLogrus silences k3d's logrus output and forwards all entries to zap.
func RedirectLogrus(zapLog *zap.Logger) {
	k3dlog.Logger.SetOutput(io.Discard)
	k3dlog.Logger.SetLevel(logrus.TraceLevel)
	k3dlog.Logger.ReplaceHooks(make(logrus.LevelHooks))
	k3dlog.Logger.AddHook(&zapHook{zap: zapLog.Named("k3d")})
}
