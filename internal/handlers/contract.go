//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package handlers

import "gopkg.in/telebot.v3"

type subHandler interface {
	Slug() string
	Handle(telebot.Context) error
}
