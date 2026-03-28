package api

import (
	"context"

	"merkle-log/server/internal/contracts"
	"merkle-log/server/internal/event"
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
)

type EventReader interface {
	ReadRawByIndex(ctx context.Context, idx uint64) ([]byte, error)
	ReadEventByIndex(ctx context.Context, idx uint64) (event.PreparedEvent, error)
}

type ProofService interface {
	InclusionProof(ctx context.Context, idx uint64) (contracts.InclusionProof, error)
	ConsistencyProof(ctx context.Context, from, to uint64) (contracts.ConsistencyProof, error)
}

type Handler struct {
	appender                *tessera.Appender
	indexer                 *index.Indexer
	eventReader             EventReader
	proofService            ProofService
	useIssuedAtAsRecordedAt bool
}

func NewHandler(a *tessera.Appender, i *index.Indexer, er EventReader, ps ProofService) *Handler {
	return &Handler{
		appender:     a,
		indexer:      i,
		eventReader:  er,
		proofService: ps,
	}
}

func (h *Handler) SetDevMode(enabled bool) {
	h.useIssuedAtAsRecordedAt = enabled
}
