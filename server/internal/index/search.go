package index

import (
	"strings"
	"time"
)

type SearchParams struct {
	DocID          string
	IssuerEntityID string
	FromInclusive  time.Time
	ToExclusive    time.Time
	Limit          int
	Offset         int
	Order          string
}

type SearchResult struct {
	Indexes    []uint64
	TotalCount int
}

func (idx *Indexer) SearchIndexes(params SearchParams) (SearchResult, error) {
	baseQuery := `
		FROM notary_index
		WHERE 1=1`
	args := make([]any, 0, 6)

	if docID := strings.TrimSpace(params.DocID); docID != "" {
		baseQuery += ` AND doc_id = ?`
		args = append(args, docID)
	}
	if issuer := strings.TrimSpace(params.IssuerEntityID); issuer != "" {
		baseQuery += ` AND issuer_entity_id = ?`
		args = append(args, issuer)
	}
	if !params.FromInclusive.IsZero() {
		baseQuery += ` AND recorded_at_unix_ns >= ?`
		args = append(args, params.FromInclusive.UnixNano())
	}
	if !params.ToExclusive.IsZero() {
		baseQuery += ` AND recorded_at_unix_ns < ?`
		args = append(args, params.ToExclusive.UnixNano())
	}

	var totalCount int
	if err := idx.db.QueryRow(`SELECT COUNT(*) `+baseQuery, args...).Scan(&totalCount); err != nil {
		return SearchResult{}, err
	}

	order := "ASC"
	if strings.EqualFold(strings.TrimSpace(params.Order), "desc") {
		order = "DESC"
	}

	query := `SELECT log_index ` + baseQuery + ` ORDER BY log_index ` + order
	queryArgs := append([]any(nil), args...)
	if params.Limit > 0 {
		query += ` LIMIT ?`
		queryArgs = append(queryArgs, params.Limit)
	} else if params.Offset > 0 {
		query += ` LIMIT -1`
	}
	if params.Offset > 0 {
		query += ` OFFSET ?`
		queryArgs = append(queryArgs, params.Offset)
	}

	rows, err := idx.db.Query(query, queryArgs...)
	if err != nil {
		return SearchResult{}, err
	}
	defer rows.Close()

	indexes := make([]uint64, 0)
	for rows.Next() {
		var i int64
		if err := rows.Scan(&i); err != nil {
			return SearchResult{}, err
		}
		if i < 0 {
			continue
		}
		indexes = append(indexes, uint64(i))
	}
	if err := rows.Err(); err != nil {
		return SearchResult{}, err
	}

	return SearchResult{
		Indexes:    indexes,
		TotalCount: totalCount,
	}, nil
}
