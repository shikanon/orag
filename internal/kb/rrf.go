package kb

import "sort"

func RRF(resultSets [][]SearchResult, k int, topN int) []SearchResult {
	if k <= 0 {
		k = 60
	}
	type acc struct {
		result SearchResult
		score  float64
	}
	seen := map[string]*acc{}
	for _, results := range resultSets {
		for i, result := range results {
			rank := result.Rank
			if rank <= 0 {
				rank = i + 1
			}
			score := 1.0 / float64(k+rank)
			item, ok := seen[result.Chunk.ID]
			if !ok {
				cp := result
				cp.Score = 0
				seen[result.Chunk.ID] = &acc{result: cp}
				item = seen[result.Chunk.ID]
			}
			item.score += score
		}
	}
	out := make([]SearchResult, 0, len(seen))
	for _, item := range seen {
		item.result.Score = item.score
		item.result.From = "rrf"
		out = append(out, item.result)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Chunk.ID < out[j].Chunk.ID
		}
		return out[i].Score > out[j].Score
	})
	for i := range out {
		out[i].Rank = i + 1
	}
	if topN > 0 && len(out) > topN {
		return out[:topN]
	}
	return out
}
