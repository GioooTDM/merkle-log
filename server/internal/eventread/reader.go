package eventread

import (
	"context"

	"merkle-log/server/internal/event"
	"merkle-log/server/internal/logread"

	"github.com/transparency-dev/tessera"
)

type Record struct {
	Raw   []byte
	Event event.PreparedEvent
}

type Reader struct {
	log tessera.LogReader
}

func New(log tessera.LogReader) *Reader {
	return &Reader{log: log}
}

func (r *Reader) ReadRawByIndex(ctx context.Context, idx uint64) ([]byte, error) {
	size, err := logread.PublishedTreeSize(ctx, r.log)
	if err != nil {
		return nil, err
	}
	return logread.ReadLogEntryByIndex(ctx, r.log, size, idx)
}

func (r *Reader) ReadRecordByIndex(ctx context.Context, idx uint64) (Record, error) {
	raw, err := r.ReadRawByIndex(ctx, idx)
	if err != nil {
		return Record{}, err
	}

	parsed, err := event.DecodePreparedEvent(raw)
	if err != nil {
		return Record{}, err
	}

	return Record{
		Raw:   raw,
		Event: parsed,
	}, nil
}
