// +heroku install cmd/main.go
module github.com/reijo1337/ToxicBot

go 1.22

require (
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/mb-14/gomarkov v0.0.0-20210216094942-a5b484cc0243
	github.com/sirupsen/logrus v1.9.3
	go.uber.org/mock v0.4.0
	golang.org/x/oauth2 v0.20.0
	gopkg.in/Iwark/spreadsheet.v2 v2.0.0-20230915040305-7677e8164883
	gopkg.in/telebot.v3 v3.2.1
)

require (
	cloud.google.com/go/compute/metadata v0.3.0 // indirect
	golang.org/x/sys v0.1.0 // indirect
)
