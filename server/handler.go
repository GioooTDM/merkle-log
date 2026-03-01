package main

import (
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
)

type NotaryHandler struct {
	appender *tessera.Appender
	indexer  *index.Indexer
	reader   tessera.LogReader
}

func NewNotaryHandler(a *tessera.Appender, i *index.Indexer, r tessera.LogReader) *NotaryHandler {
	return &NotaryHandler{appender: a, indexer: i, reader: r}
}
