package extract

import (
	"fmt"
	"log/slog"

	"github.com/dleandro/transfer-scout-api/internal/config"
)

// NewFromConfig builds the Extractor the extract stage runs with;
// pipeline.NewExtractor (used by cmd/pipeline and cmd/extract) delegates here.
//
// With no EXTRACT_API_KEY it is the stub, as before. With one, it is Claude
// behind the Jev gate — and it refuses to build without TYPESAFE_API_KEY
// rather than run Claude ungated: paying Claude for articles that are not
// rumours is the cost the gate exists to remove, and a Warn line in a
// scheduled job's logs is not something anyone reads before the bill.
func NewFromConfig(cfg config.Config) (Extractor, error) {
	if cfg.ExtractAPIKey == "" {
		slog.Warn("extract: EXTRACT_API_KEY not set, using stub extractor — no articles will actually be extracted")
		return StubExtractor{}, nil
	}
	if cfg.TypesafeAPIKey == "" {
		return nil, fmt.Errorf("extract: EXTRACT_API_KEY is set but TYPESAFE_API_KEY is not; refusing to call Claude without the Jev gate")
	}

	claude := NewAnthropicExtractor(cfg.ExtractAPIKey, cfg.ExtractModel)
	if cfg.ExtractBaseURL != "" {
		claude.BaseURL = cfg.ExtractBaseURL
	}
	return &GatedExtractor{
		Classifier:     NewJevClassifierFromConfig(cfg),
		Extractor:      claude,
		MinProbability: cfg.JevMinProbability,
	}, nil
}

// NewJevClassifierFromConfig builds the Jev classifier on its own, for
// callers that classify without extracting (cmd/reclassify).
func NewJevClassifierFromConfig(cfg config.Config) *JevClassifier {
	jev := NewJevClassifier(cfg.TypesafeAPIKey, cfg.JevModel)
	if cfg.JevBaseURL != "" {
		jev.BaseURL = cfg.JevBaseURL
	}
	return jev
}
