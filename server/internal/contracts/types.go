package contracts

type InclusionProof struct {
	LogIndex   uint64   `json:"log_index"`
	TreeSize   uint64   `json:"tree_size"`
	RootHash   string   `json:"root_hash"`
	Checkpoint string   `json:"checkpoint"`
	Proof      []string `json:"proof"`
}

type ConsistencyProof struct {
	FromTreeSize uint64   `json:"from_tree_size"`
	ToTreeSize   uint64   `json:"to_tree_size"`
	Proof        []string `json:"proof"`
}
