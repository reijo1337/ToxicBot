package message

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type LLMMessage struct {
	Role    Role
	Name    string
	Content string
}
