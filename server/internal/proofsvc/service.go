package proofsvc

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"merkle-log/server/internal/logread"

	"github.com/transparency-dev/tessera"
	tclient "github.com/transparency-dev/tessera/client"
)

var (
	ErrCheckpointUnavailable = errors.New("checkpoint not available")
	ErrIndexOutOfRange       = errors.New("index out of range")
	ErrSizeOutOfRange        = errors.New("requested size is beyond published checkpoint")
)

type InclusionProofResult struct {
	LogIndex   uint64   `json:"log_index"`
	TreeSize   uint64   `json:"tree_size"`
	RootHash   string   `json:"root_hash"`
	Checkpoint string   `json:"checkpoint"`
	Proof      []string `json:"proof"`
}

type ConsistencyProofResult struct {
	FromTreeSize uint64   `json:"from_tree_size"`
	ToTreeSize   uint64   `json:"to_tree_size"`
	Proof        []string `json:"proof"`
}

type Service struct {
	reader tessera.LogReader
}

func New(reader tessera.LogReader) *Service {
	return &Service{reader: reader}
}

func (s *Service) InclusionProof(ctx context.Context, idx uint64) (InclusionProofResult, error) {
	cpRaw, cp, err := logread.ReadPublishedCheckpoint(ctx, s.reader)
	if err != nil {
		return InclusionProofResult{}, fmt.Errorf("%w: %v", ErrCheckpointUnavailable, err)
	}
	if idx >= cp.Size {
		return InclusionProofResult{}, ErrIndexOutOfRange
	}

	pb, err := tclient.NewProofBuilder(ctx, cp.Size, s.tileFetcher())
	if err != nil {
		return InclusionProofResult{}, err
	}

	hashes, err := pb.InclusionProof(ctx, idx)
	if err != nil {
		return InclusionProofResult{}, err
	}

	return InclusionProofResult{
		LogIndex:   idx,
		TreeSize:   cp.Size,
		RootHash:   hex.EncodeToString(cp.Hash),
		Checkpoint: string(cpRaw),
		Proof:      encodeHashes(hashes),
	}, nil
}

func (s *Service) ConsistencyProof(ctx context.Context, from, to uint64) (ConsistencyProofResult, error) {
	_, cp, err := logread.ReadPublishedCheckpoint(ctx, s.reader)
	if err != nil {
		return ConsistencyProofResult{}, fmt.Errorf("%w: %v", ErrCheckpointUnavailable, err)
	}
	if to > cp.Size {
		return ConsistencyProofResult{}, ErrSizeOutOfRange
	}

	pb, err := tclient.NewProofBuilder(ctx, cp.Size, s.tileFetcher())
	if err != nil {
		return ConsistencyProofResult{}, err
	}

	hashes, err := pb.ConsistencyProof(ctx, from, to)
	if err != nil {
		return ConsistencyProofResult{}, err
	}

	return ConsistencyProofResult{
		FromTreeSize: from,
		ToTreeSize:   to,
		Proof:        encodeHashes(hashes),
	}, nil
}

func encodeHashes(hashes [][]byte) []string {
	proofHex := make([]string, len(hashes))
	for i := range hashes {
		proofHex[i] = hex.EncodeToString(hashes[i])
	}
	return proofHex
}

// tileFetcher returns a TileFetcherFunc compatible with tessera client proof builder.
// If partial tiles are unavailable, it falls back to the corresponding full tile.
func (s *Service) tileFetcher() func(ctx context.Context, level, index uint64, p uint8) ([]byte, error) {
	return func(ctx context.Context, level, index uint64, p uint8) ([]byte, error) {
		b, err := s.reader.ReadTile(ctx, level, index, p)
		if err == nil {
			return b, nil
		}
		if p != 0 {
			b2, err2 := s.reader.ReadTile(ctx, level, index, 0)
			if err2 == nil {
				return b2, nil
			}
			if errors.Is(err2, os.ErrNotExist) {
				return nil, os.ErrNotExist
			}
			return nil, err2
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
}
