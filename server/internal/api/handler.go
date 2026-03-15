package api

import (
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
)

type Handler struct {
	appender                *tessera.Appender
	indexer                 *index.Indexer
	reader                  tessera.LogReader
	useIssuedAtAsRecordedAt bool
}

func NewHandler(a *tessera.Appender, i *index.Indexer, r tessera.LogReader) *Handler {
	return &Handler{appender: a, indexer: i, reader: r}
}

func (h *Handler) SetDevMode(enabled bool) {
	h.useIssuedAtAsRecordedAt = enabled
}
