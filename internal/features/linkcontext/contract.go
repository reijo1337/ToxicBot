package linkcontext

import (
	"context"

	"github.com/reijo1337/ToxicBot/internal/infrastructure/linkpreview"
)

type Reference struct {
	URL    string
	Anchor string
}

type pageLoader interface {
	Page(ctx context.Context, rawURL string) (linkpreview.Metadata, error)
	Image(ctx context.Context, rawURL string) ([]byte, error)
}

type imageDescriber interface {
	GenerateContent(ctx context.Context, prompt string, imageBytes []byte) (string, error)
}

type logger interface {
	Warn(context.Context, string)
}
