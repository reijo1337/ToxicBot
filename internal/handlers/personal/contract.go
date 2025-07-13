//go:generate go tool go.uber.org/mock/mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package personal

type messageRepository interface {
	GetEnabledMessages() ([]string, error)
}
