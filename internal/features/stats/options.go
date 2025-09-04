package stats

import "github.com/reijo1337/ToxicBot/internal/message"

type option struct {
	extra *ResponseExtra
}

type Option func(o *option)

func WithGenStrategy(genStrategy message.GenerationStrategy) Option {
	return func(o *option) {
		if o.extra == nil {
			o.extra = &ResponseExtra{}
		}
		o.extra.TextGenerationType = genStrategy
	}
}
