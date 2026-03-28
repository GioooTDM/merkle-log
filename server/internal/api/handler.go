package api

import (
	"context"

	"merkle-log/server/internal/eventread"
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
)

type EventReader interface {
	ReadRawByIndex(ctx context.Context, idx uint64) ([]byte, error)
	ReadRecordByIndex(ctx context.Context, idx uint64) (eventread.Record, error)
}

type Handler struct {
	appender                *tessera.Appender
	indexer                 *index.Indexer
	reader                  tessera.LogReader
	eventReader             EventReader
	useIssuedAtAsRecordedAt bool
}

func NewHandler(a *tessera.Appender, i *index.Indexer, r tessera.LogReader, er EventReader) *Handler {
	return &Handler{
		appender:    a,
		indexer:     i,
		reader:      r,
		eventReader: er,
	}
}

func (h *Handler) SetDevMode(enabled bool) {
	h.useIssuedAtAsRecordedAt = enabled
}
