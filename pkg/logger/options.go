package logger

import "github.com/reijo1337/ToxicBot/pkg/pointer"

type Level uint8

const (
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel Level = iota
	// InfoLevel level. General operational entries about what's going on inside the
	// application.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
)

type Format uint8

const (
	TextFormat Format = iota
	JsonFormat
)

type options struct {
	level        *Level
	format       *Format
	reportCaller *bool
}

func makeDefaultUnsetedOptions(o *options) {
	if o.level == nil {
		o.level = pointer.To(InfoLevel)
	}
	if o.format == nil {
		o.format = pointer.To(TextFormat)
	}
	if o.reportCaller == nil {
		o.reportCaller = pointer.To(false)
	}
}

type Option func(o *options)

func WithLogLever(level Level) Option {
	return func(o *options) {
		o.level = &level
	}
}

func WithFormat(format Format) Option {
	return func(o *options) {
		o.format = &format
	}
}

func WithReposrtCaller(reportCaller bool) Option {
	return func(o *options) {
		o.reportCaller = &reportCaller
	}
}
