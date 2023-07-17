package message

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mb-14/gomarkov"
)

type Generator struct {
	storage      messageRepository
	r            randomizer
	logger       logger
	chain        *gomarkov.Chain
	messages     []string
	updatePeriod time.Duration
	mu           sync.RWMutex
	markovChance float32
}

func New(
	ctx context.Context,
	s messageRepository,
	logger logger,
	r randomizer,
	updatePeriod time.Duration,
	markovChance float32,
) (*Generator, error) {
	out := Generator{
		storage:      s,
		logger:       logger,
		r:            r,
		chain:        gomarkov.NewChain(1),
		updatePeriod: updatePeriod,
		markovChance: markovChance,
	}

	if err := out.reloadMessages(); err != nil {
		return nil, fmt.Errorf("cannot load messages: %w", err)
	}

	go out.runUpdater(ctx)

	return &out, nil
}

func (g *Generator) runUpdater(ctx context.Context) {
	t := time.NewTimer(g.updatePeriod)
	for {
		select {
		case <-t.C:
			if err := g.reloadMessages(); err != nil {
				g.logger.Warn(
					g.logger.WithError(ctx, err),
					"cannot reload messages",
				)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (g *Generator) reloadMessages() error {
	r, err := g.storage.GetEnabledRandom()
	if err != nil {
		return err
	}

	m := make([]string, len(r))
	copy(m, r)

	chain := gomarkov.NewChain(1)
	for _, message := range m {
		chain.Add(strings.Split(strings.Trim(message, " "), " "))
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.messages = m
	g.chain = chain

	return nil
}

func (g *Generator) GetMessageText() string {
	if g.r.Float32() <= g.markovChance {
		text, err := g.generateDegenerate()
		if err == nil {
			return text
		}
	}

	g.mu.RLock()
	defer g.mu.RUnlock()
	randomIndex := g.r.Intn(len(g.messages))
	return g.messages[randomIndex]
}

func (g *Generator) generateDegenerate() (string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	tokens := []string{gomarkov.StartToken}
	for tokens[len(tokens)-1] != gomarkov.EndToken {
		next, err := g.chain.Generate(tokens[(len(tokens) - 1):])
		if err != nil {
			return "", fmt.Errorf("can't generate next token: %w", err)
		}
		tokens = append(tokens, next)
	}
	return strings.Join(tokens[1:len(tokens)-1], " "), nil
}
