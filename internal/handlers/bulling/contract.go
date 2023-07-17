//go:generate mockgen -source $GOFILE -destination mocks_test.go -package ${GOPACKAGE}
package bulling

type messageGenerator interface {
	GetMessageText() string
}
