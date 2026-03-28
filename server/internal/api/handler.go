package api

import (
	"context"

	"merkle-log/server/internal/eventread"
	"merkle-log/server/internal/index"
	"merkle-log/server/internal/proofsvc"

	"github.com/transparency-dev/tessera"
)

type EventReader interface {
	ReadRawByIndex(ctx context.Context, idx uint64) ([]byte, error)
	ReadRecordByIndex(ctx context.Context, idx uint64) (eventread.Record, error)
}

type ProofService interface {
	InclusionProof(ctx context.Context, idx uint64) (proofsvc.InclusionProofResult, error)
	ConsistencyProof(ctx context.Context, from, to uint64) (proofsvc.ConsistencyProofResult, error)
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
