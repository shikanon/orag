package eval

import (
	"math"

	"github.com/shikanon/orag/internal/kb"
)

const defaultAlphaNDCGPenalty = 0.5

// DiversityAnnotation marks which retrieved evidence can satisfy an aspect or subquestion.
type DiversityAnnotation struct {
	Aspect      string   `json:"aspect,omitempty"`
	Subquestion string   `json:"subquestion,omitempty"`
	ChunkID     string   `json:"chunk_id,omitempty"`
	ChunkIDs    []string `json:"chunk_ids,omitempty"`
	DocumentID  string   `json:"document_id,omitempty"`
	DocumentIDs []string `json:"document_ids,omitempty"`
	SourceURI   string   `json:"source_uri,omitempty"`
	SourceURIs  []string `json:"source_uris,omitempty"`
}

type DiversityOptions struct {
	// Alpha controls repeated aspect gain decay. Values outside (0, 1) use 0.5.
	Alpha float64
	K     int
}

func ScoreDiversity(results []kb.SearchResult, annotations []DiversityAnnotation, opts DiversityOptions) (map[string]float64, bool) {
	labelsByRef, allLabels := diversityLabelsByRef(annotations)
	if len(allLabels) == 0 {
		return nil, false
	}

	k := opts.K
	if k <= 0 || k > len(results) {
		k = len(results)
	}
	rankedLabels := make([]map[string]struct{}, 0, k)
	covered := map[string]struct{}{}
	for _, result := range results[:k] {
		labels := labelsForSearchResult(result, labelsByRef)
		for label := range labels {
			covered[label] = struct{}{}
		}
		rankedLabels = append(rankedLabels, labels)
	}

	idealCandidates := make([]map[string]struct{}, 0, len(labelsByRef))
	for _, labels := range labelsByRef {
		idealCandidates = append(idealCandidates, labels)
	}
	alpha := opts.Alpha
	if alpha <= 0 || alpha >= 1 {
		alpha = defaultAlphaNDCGPenalty
	}
	ideal := greedyIdealAlphaDCG(idealCandidates, k, alpha)
	ndcg := 0.0
	if ideal > 0 {
		ndcg = alphaDCG(rankedLabels, alpha) / ideal
	}
	return map[string]float64{
		"alpha_ndcg":      ndcg,
		"aspect_coverage": float64(len(covered)) / float64(len(allLabels)),
	}, true
}

func diversityLabelsByRef(annotations []DiversityAnnotation) (map[string]map[string]struct{}, map[string]struct{}) {
	labelsByRef := map[string]map[string]struct{}{}
	allLabels := map[string]struct{}{}
	for _, annotation := range annotations {
		label := annotation.Aspect
		if label == "" {
			label = annotation.Subquestion
		}
		if label == "" {
			continue
		}
		refs := diversityRefs(annotation)
		if len(refs) == 0 {
			continue
		}
		allLabels[label] = struct{}{}
		for _, ref := range refs {
			if _, ok := labelsByRef[ref]; !ok {
				labelsByRef[ref] = map[string]struct{}{}
			}
			labelsByRef[ref][label] = struct{}{}
		}
	}
	return labelsByRef, allLabels
}

func diversityRefs(annotation DiversityAnnotation) []string {
	var refs []string
	if annotation.ChunkID != "" {
		refs = append(refs, "chunk:"+annotation.ChunkID)
	}
	for _, id := range annotation.ChunkIDs {
		if id != "" {
			refs = append(refs, "chunk:"+id)
		}
	}
	if annotation.DocumentID != "" {
		refs = append(refs, "doc:"+annotation.DocumentID)
	}
	for _, id := range annotation.DocumentIDs {
		if id != "" {
			refs = append(refs, "doc:"+id)
		}
	}
	if annotation.SourceURI != "" {
		refs = append(refs, "source:"+annotation.SourceURI)
	}
	for _, uri := range annotation.SourceURIs {
		if uri != "" {
			refs = append(refs, "source:"+uri)
		}
	}
	return refs
}

func labelsForSearchResult(result kb.SearchResult, labelsByRef map[string]map[string]struct{}) map[string]struct{} {
	labels := map[string]struct{}{}
	refs := []string{
		"chunk:" + result.Chunk.ID,
		"doc:" + result.Chunk.DocumentID,
		"source:" + result.Chunk.SourceURI,
	}
	for _, ref := range refs {
		for label := range labelsByRef[ref] {
			labels[label] = struct{}{}
		}
	}
	return labels
}

func alphaDCG(rankedLabels []map[string]struct{}, alpha float64) float64 {
	seen := map[string]int{}
	var dcg float64
	for i, labels := range rankedLabels {
		var gain float64
		for label := range labels {
			gain += math.Pow(1-alpha, float64(seen[label]))
		}
		for label := range labels {
			seen[label]++
		}
		rank := float64(i + 1)
		dcg += gain / math.Log2(rank+1)
	}
	return dcg
}

func greedyIdealAlphaDCG(candidates []map[string]struct{}, k int, alpha float64) float64 {
	if k <= 0 || len(candidates) == 0 {
		return 0
	}
	used := make([]bool, len(candidates))
	seen := map[string]int{}
	var dcg float64
	for rank := 1; rank <= k; rank++ {
		bestIdx := -1
		bestGain := 0.0
		for idx, labels := range candidates {
			if used[idx] {
				continue
			}
			gain := alphaGain(labels, seen, alpha)
			if gain > bestGain {
				bestGain = gain
				bestIdx = idx
			}
		}
		if bestIdx == -1 {
			break
		}
		used[bestIdx] = true
		for label := range candidates[bestIdx] {
			seen[label]++
		}
		dcg += bestGain / math.Log2(float64(rank)+1)
	}
	return dcg
}

func alphaGain(labels map[string]struct{}, seen map[string]int, alpha float64) float64 {
	var gain float64
	for label := range labels {
		gain += math.Pow(1-alpha, float64(seen[label]))
	}
	return gain
}
