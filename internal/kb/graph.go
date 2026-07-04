package kb

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type GraphRetriever struct {
	Base  Retriever
	Store GraphStore
	TopK  int
}

func (r GraphRetriever) Retrieve(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	var base []SearchResult
	var err error
	if r.Base != nil {
		base, err = r.Base.Retrieve(ctx, req)
		if err != nil {
			return nil, err
		}
	}
	if r.Store == nil {
		return base, nil
	}
	limit := req.TopK
	if limit <= 0 {
		limit = r.TopK
	}
	if limit <= 0 {
		limit = 8
	}
	entities := ExtractGraphEntities(req.Query, 8)
	graphResults, err := r.Store.ExpandGraph(ctx, GraphExpansionRequest{
		TenantID:        req.TenantID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		Entities:        entities,
		Limit:           limit,
	})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]SearchResult, 0, len(base)+len(graphResults))
	for _, result := range base {
		seen[result.Chunk.ID] = true
		out = append(out, result)
	}
	for _, result := range graphResults {
		if seen[result.Chunk.ID] {
			continue
		}
		result.From = "graph"
		out = append(out, result)
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	if len(out) > limit {
		return out[:limit], nil
	}
	return out, nil
}

var graphEntityPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_\-]{2,}`)

func ExtractGraphEntities(text string, limit int) []string {
	seen := map[string]bool{}
	var out []string
	for _, match := range graphEntityPattern.FindAllString(text, -1) {
		addGraphEntity(&out, seen, match, limit)
	}
	var current strings.Builder
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.IsDigit(r) {
			current.WriteRune(r)
			continue
		}
		flushGraphEntity(&out, seen, &current, limit)
	}
	flushGraphEntity(&out, seen, &current, limit)
	sort.SliceStable(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func flushGraphEntity(out *[]string, seen map[string]bool, current *strings.Builder, limit int) {
	value := current.String()
	current.Reset()
	if len([]rune(value)) < 2 {
		return
	}
	addGraphEntity(out, seen, value, limit)
}

func addGraphEntity(out *[]string, seen map[string]bool, value string, limit int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := NormalizeQuery(value)
	if key == "" || seen[key] {
		return
	}
	seen[key] = true
	*out = append(*out, value)
}
