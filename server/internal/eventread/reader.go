package eventread

import (
	"context"

	"merkle-log/server/internal/event"
	"merkle-log/server/internal/logread"

	"github.com/transparency-dev/tessera"
)

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

func (r *Reader) ReadEventByIndex(ctx context.Context, idx uint64) (event.PreparedEvent, error) {
	raw, err := r.ReadRawByIndex(ctx, idx)
	if err != nil {
		return event.PreparedEvent{}, err
	}

	parsed, err := event.DecodePreparedEvent(raw)
	if err != nil {
		return event.PreparedEvent{}, err
	}

	return parsed, nil
}
