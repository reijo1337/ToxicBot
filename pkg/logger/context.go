package logger

import (
	"context"

	"github.com/sirupsen/logrus"
)

var (
	entryCtxKey struct{}
)

func (l *Logger) getEntry(ctx context.Context) *logrus.Entry {
	entry, ok := ctx.Value(entryCtxKey).(*logrus.Entry)
	if !ok {
		entry = logrus.NewEntry(l.l)
	}

	return entry
}

func (l *Logger) ctxBuilder(ctx context.Context, buildEntry func(*logrus.Entry) *logrus.Entry) context.Context {
	entry := l.getEntry(ctx)
	entry = buildEntry(entry)
	return context.WithValue(ctx, entryCtxKey, entry)
}

func (l *Logger) WithError(ctx context.Context, err error) context.Context {
	return l.ctxBuilder(ctx, func(e *logrus.Entry) *logrus.Entry { return e.WithError(err) })
}

func (l *Logger) WithField(ctx context.Context, key string, value any) context.Context {
	return l.ctxBuilder(ctx, func(e *logrus.Entry) *logrus.Entry { return e.WithField(key, value) })
}

func (l *Logger) WithFields(ctx context.Context, fileds map[string]any) context.Context {
	return l.ctxBuilder(ctx, func(e *logrus.Entry) *logrus.Entry { return e.WithFields(fileds) })
}
