package main

import (
	"github.com/reijo1337/ToxicBot/internal/app"
	"github.com/reijo1337/ToxicBot/internal/handlers/bulling"
	"github.com/reijo1337/ToxicBot/internal/handlers/greetings"
	"github.com/reijo1337/ToxicBot/internal/handlers/igor"
	"github.com/reijo1337/ToxicBot/internal/handlers/leave"

	"github.com/sirupsen/logrus"
)

func main() {
	logger := newLogger()
	a, err := app.New(app.WithLogger(logger))
	if err != nil {
		logger.WithError(err).Fatal("init application")
	}

	greetingsHandler, err := greetings.New()
	if err != nil {
		logger.WithError(err).Fatal("init greetings handler")
	}

	igorHandler, err := igor.New()
	if err != nil {
		logger.WithError(err).Fatal("init igor handler")
	}

	bullingHandler, err := bulling.New()
	if err != nil {
		logger.WithError(err).Fatal("init bulling handler")
	}

	a.RegisterHandler(greetingsHandler.Handler)
	a.RegisterHandler(leave.Handler)
	a.RegisterHandler(igorHandler.Handler)
	a.RegisterHandler(bullingHandler.Handler)

	a.Run()
}

func newLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{})
	logger.SetReportCaller(true)

	return logger
}
