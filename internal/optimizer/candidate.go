package optimizer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type CandidateConfig struct {
	ID        string             `json:"id,omitempty"`
	Hash      string             `json:"hash,omitempty"`
	Prompt    PromptCandidate    `json:"prompt,omitempty"`
	Chunking  ChunkingCandidate  `json:"chunking,omitempty"`
	Embedding EmbeddingCandidate `json:"embedding,omitempty"`
	Reranker  RerankerCandidate  `json:"reranker,omitempty"`
	Retrieval RetrievalCandidate `json:"retrieval,omitempty"`
	Indexing  IndexingCandidate  `json:"indexing,omitempty"`
	Graph     GraphCandidate     `json:"graph,omitempty"`
	Harness   HarnessCandidate   `json:"harness,omitempty"`
}

type PromptCandidate struct {
	Name   string `json:"name,omitempty"`
	System string `json:"system,omitempty"`
}

type ChunkingCandidate struct {
	Enabled       bool   `json:"enabled"`
	SizeTokens    int    `json:"size_tokens,omitempty"`
	OverlapTokens int    `json:"overlap_tokens,omitempty"`
	ParserMethod  string `json:"parser_method,omitempty"`
}

type EmbeddingCandidate struct {
	Enabled    bool   `json:"enabled"`
	Model      string `json:"model,omitempty"`
	Dimensions int    `json:"dimensions,omitempty"`
}

type RerankerCandidate struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	TopN     int    `json:"top_n,omitempty"`
}

type RetrievalCandidate struct {
	DenseTopK              int     `json:"dense_top_k,omitempty"`
	SparseTopK             int     `json:"sparse_top_k,omitempty"`
	RRFK                   int     `json:"rrf_k,omitempty"`
	SemanticCacheThreshold float64 `json:"semantic_cache_threshold,omitempty"`
}

type IndexingCandidate struct {
	Namespace string `json:"namespace,omitempty"`
}

type GraphCandidate struct {
	QueryRewriteEnabled *bool    `json:"query_rewrite_enabled,omitempty"`
	HyDEEnabled         *bool    `json:"hyde_enabled,omitempty"`
	MultiQueryCount     int      `json:"multi_query_count,omitempty"`
	Modules             []string `json:"modules,omitempty"`
}

type HarnessCandidate struct {
	Kind    string   `json:"kind,omitempty"`
	Command string   `json:"command,omitempty"`
	Argv    []string `json:"argv,omitempty"`
}

func (c CandidateConfig) WithDeterministicID(runID string) CandidateConfig {
	c.ID = ""
	c.Hash = ""
	payload, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	configSum := sha256.Sum256(payload)
	c.Hash = hex.EncodeToString(configSum[:])

	idPayload := append([]byte(runID), ':')
	idPayload = append(idPayload, c.Hash...)
	idSum := sha256.Sum256(idPayload)
	c.ID = "cand_" + hex.EncodeToString(idSum[:])[:24]
	return c
}
