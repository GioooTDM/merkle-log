package api

import (
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
)

type NotaryHandler struct {
	appender                *tessera.Appender
	indexer                 *index.Indexer
	reader                  tessera.LogReader
	useIssuedAtAsRecordedAt bool
}

func NewNotaryHandler(a *tessera.Appender, i *index.Indexer, r tessera.LogReader) *NotaryHandler {
	return &NotaryHandler{appender: a, indexer: i, reader: r}
}

func (h *NotaryHandler) SetDevMode(enabled bool) {
	h.useIssuedAtAsRecordedAt = enabled
}
