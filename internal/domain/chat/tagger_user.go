package chat

// TaggerUser — известный тегеру участник чата с последней подписью для модели.
type TaggerUser struct {
	Author string
	ChatID int64
	UserID int64
}
