//go:generate mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package personal

type messageRepository interface {
	GetEnabledMessages() ([]string, error)
}
