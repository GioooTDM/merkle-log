package main

import (
	"github.com/transparency-dev/tessera"
)

type NotaryHandler struct {
	appender *tessera.Appender
	indexer  *Indexer
	reader   tessera.LogReader
}

func NewNotaryHandler(a *tessera.Appender, i *Indexer, r tessera.LogReader) *NotaryHandler {
	return &NotaryHandler{appender: a, indexer: i, reader: r}
}
