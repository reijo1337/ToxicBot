package stats

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"
)

const base = 10

type Stats struct {
	storage storage
	logger  logger
	block   cipher.Block
}

func New(aesKeyString string, storage storage, logger logger) (*Stats, error) {
	key, err := base64.RawStdEncoding.DecodeString(aesKeyString)
	if err != nil {
		return nil, fmt.Errorf("invalid key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("can't create aes cipher block: %w", err)
	}
	return &Stats{
		storage: storage,
		logger:  logger,
		block:   block,
	}, nil
}

func (s *Stats) Inc(ctx context.Context, chatID, userID int64, op OperationType, opts ...Option) {
	var opt option
	for _, o := range opts {
		o(&opt)
	}

	event := Response{
		Date:          time.Now().UTC(),
		OperationType: op,
		ChatIDHash:    s.encrypt(chatID),
		UserIDHash:    s.encrypt(userID),
		Extra:         opt.extra,
	}

	err := s.storage.Create(ctx, event)
	if err != nil {
		s.logger.Warn(
			s.logger.WithError(
				s.logger.WithFields(ctx, map[string]any{
					"op": op,
				}),
				err,
			),
			"can't save event",
		)
	}
}

func (s *Stats) encrypt(id int64) []byte {
	in := []byte(strconv.FormatInt(id, base))
	diff := s.block.BlockSize() - len(in)
	if diff > 0 {
		tmp := make([]byte, s.block.BlockSize())
		copy(tmp[diff:], in)
		in = tmp
	}

	out := make([]byte, len(in))

	s.block.Encrypt(out, in)

	return out
}
