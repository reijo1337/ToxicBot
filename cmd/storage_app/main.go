package main

import (
	"context"
	"github.com/reijo1337/ToxicBot/internal/google_spreadsheet"
	"github.com/reijo1337/ToxicBot/internal/storage"
	"github.com/sirupsen/logrus"
)

func main() {
	client, err := google_spreadsheet.New(context.Background())
	if err != nil {
		logrus.Fatal(err)
	}

	s := storage.New(client)

	gr, err := s.GetVoices()
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Info(gr)
}
