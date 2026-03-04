package api

import (
	"net/http"

	"merkle-log/server/internal/anchor"
)

func RegisterRoutes(mux *http.ServeMux, h *NotaryHandler, anchorWorker *anchor.Worker) {
	mux.HandleFunc("/add", h.AddEvent)
	mux.HandleFunc("/get-by-doc", h.GetByDoc)
	mux.HandleFunc("/get-by-leaf", h.GetByLeaf)
	mux.HandleFunc("/get-entry/", h.GetEntry)
	mux.HandleFunc("/get-proof/", h.GetProof)
	mux.HandleFunc("/get-consistency", h.GetConsistencyProof)
	mux.HandleFunc("/get-indexes", h.GetIndexesByDocUID)
	mux.HandleFunc("/get-entries-by-docuid", h.GetEntriesByDocUID)
	mux.HandleFunc("/anchor/force", forceAnchorHandler(anchorWorker))
	mux.HandleFunc("/anchor/latest", latestAnchorHandler(anchorWorker))
}
