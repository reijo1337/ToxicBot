package logger

import (
	"context"

	"github.com/sirupsen/logrus"
)

var (
	logrusLevel = map[Level]logrus.Level{
		WarnLevel:  logrus.WarnLevel,
		InfoLevel:  logrus.InfoLevel,
		DebugLevel: logrus.DebugLevel,
	}

	logrusFormater = map[Format]func() logrus.Formatter{
		TextFormat: func() logrus.Formatter { return &logrus.TextFormatter{} },
		JsonFormat: func() logrus.Formatter { return &logrus.JSONFormatter{} },
	}
)

type Logger struct {
	l *logrus.Logger
}

func New(opts ...Option) *Logger {
	options := &options{}
	for _, setOption := range opts {
		setOption(options)
	}

	makeDefaultUnsetedOptions(options)

	l := logrus.New()
	l.SetLevel(logrusLevel[*options.level])
	l.SetFormatter(logrusFormater[*options.format]())
	l.SetReportCaller(*options.reportCaller)

	return &Logger{l: l}
}

func (l *Logger) Info(ctx context.Context, msg string) {
	l.entryDo(ctx, func(e *logrus.Entry) { e.Info(msg) })
}

func (l *Logger) Warn(ctx context.Context, msg string) {
	l.entryDo(ctx, func(e *logrus.Entry) { e.Warn(msg) })
}

func (l *Logger) Debug(ctx context.Context, msg string) {
	l.entryDo(ctx, func(e *logrus.Entry) { e.Debug(msg) })
}

func (l *Logger) Error(ctx context.Context, msg string) {
	l.entryDo(ctx, func(e *logrus.Entry) { e.Error(msg) })
}

func (l *Logger) Fatal(ctx context.Context, msg string) {
	l.entryDo(ctx, func(e *logrus.Entry) { e.Fatal(msg) })
}

func (l *Logger) entryDo(ctx context.Context, do func(*logrus.Entry)) {
	entry := l.getEntry(ctx)
	do(entry)
}
