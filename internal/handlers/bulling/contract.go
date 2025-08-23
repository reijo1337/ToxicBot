//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package bulling

type messageGenerator interface {
	GetMessageText(replyTo string) string
}
