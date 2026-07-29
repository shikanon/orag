package chunkpreview

import (
	"strings"
	"unicode/utf8"

	"github.com/shikanon/orag/internal/ingest/chunker"
)

// Config controls how Preview generates chunks and what statistics are returned.
type Config struct {
	Strategy         string
	SizeTokens       int
	OverlapTokens    int
	ParentChild      bool
	ParentSizeTokens int
	ChildSizeTokens  int
	SampleCount      int
}

// PreviewResult is the read-only output of a Preview call. It includes the
// strategy that was selected, aggregate chunk statistics, a sample of chunks,
// and any warnings.
type PreviewResult struct {
	StrategySelected StrategySelection
	Stats            ChunkStats
	SampleChunks     []SampleChunk
	Warnings         []string
}

// ChunkStats holds aggregate statistics about the chunk output, suitable for
// preview UI and governance dashboards.
type ChunkStats struct {
	TotalChunks      int
	TotalTokens      int
	AvgChunkTokens   float64
	MinChunkTokens   int
	MaxChunkTokens   int
	SizeDistribution map[string]int
	OrphanChunks     int
}

// SampleChunk represents a single chunk sampled from the full chunk list for
// human preview. Content is truncated to a preview-friendly length.
type SampleChunk struct {
	Index   int
	Content string
	Section string
	Tokens  int
	Offset  int
}

// Preview profiles the given text, selects a chunking strategy, generates
// chunks, computes statistics, and returns a fully read-only preview result.
// It has no side effects.
func Preview(text string, config Config) PreviewResult {
	profile := Profile(text)
	selection := SelectStrategy(profile, config)

	size := selection.RecommendedSize
	overlap := selection.RecommendedOverlap

	var chunks []chunker.Chunk
	switch selection.Strategy {
	case StrategyHeading:
		chunks = splitByHeading(text, size, overlap)
	case StrategyHeuristic:
		chunks = splitHeuristic(text, size, overlap)
	case StrategyRecursive:
		fallthrough
	default:
		recursive := chunker.Recursive{SizeTokens: size, OverlapTokens: overlap}
		chunks = recursive.Split(text)
	}

	stats := computeStats(chunks)
	warnings := buildWarnings(profile, stats, selection)
	samples := sampleChunks(chunks, sampleCount(config.SampleCount))

	return PreviewResult{
		StrategySelected: selection,
		Stats:            stats,
		SampleChunks:     samples,
		Warnings:         warnings,
	}
}

func sampleCount(count int) int {
	if count <= 0 {
		return 3
	}
	return count
}

func computeStats(chunks []chunker.Chunk) ChunkStats {
	stats := ChunkStats{
		TotalChunks:      len(chunks),
		SizeDistribution: make(map[string]int),
		MinChunkTokens:   -1,
	}

	for _, c := range chunks {
		tokens := chunker.TokenCount(c.Content)
		stats.TotalTokens += tokens

		if stats.MinChunkTokens == -1 || tokens < stats.MinChunkTokens {
			stats.MinChunkTokens = tokens
		}
		if tokens > stats.MaxChunkTokens {
			stats.MaxChunkTokens = tokens
		}

		bucket := sizeBucket(tokens)
		stats.SizeDistribution[bucket]++

		if c.Section == "" {
			stats.OrphanChunks++
		}
	}

	if stats.TotalChunks > 0 {
		stats.AvgChunkTokens = float64(stats.TotalTokens) / float64(stats.TotalChunks)
	}
	if stats.MinChunkTokens == -1 {
		stats.MinChunkTokens = 0
	}

	return stats
}

func sizeBucket(tokens int) string {
	switch {
	case tokens < 200:
		return "<200"
	case tokens < 400:
		return "200-400"
	case tokens < 600:
		return "400-600"
	case tokens < 800:
		return "600-800"
	case tokens < 1000:
		return "800-1000"
	default:
		return ">1000"
	}
}

func sampleChunks(chunks []chunker.Chunk, count int) []SampleChunk {
	if len(chunks) == 0 || count <= 0 {
		return nil
	}

	samples := make([]SampleChunk, 0, min(count, len(chunks)))

	if len(chunks) <= count {
		for i, c := range chunks {
			samples = append(samples, toSampleChunk(i, c))
		}
		return samples
	}

	step := len(chunks) / count
	for i := 0; i < count; i++ {
		idx := i * step
		if idx >= len(chunks) {
			idx = len(chunks) - 1
		}
		samples = append(samples, toSampleChunk(idx, chunks[idx]))
	}

	return samples
}

func toSampleChunk(index int, c chunker.Chunk) SampleChunk {
	return SampleChunk{
		Index:   index,
		Content: truncatePreview(c.Content, 200),
		Section: c.Section,
		Tokens:  chunker.TokenCount(c.Content),
		Offset:  c.Offset,
	}
}

func truncatePreview(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}

	runes := []rune(s)
	truncated := string(runes[:max])
	return strings.TrimSpace(truncated) + "..."
}

func buildWarnings(profile DocumentProfile, stats ChunkStats, selection StrategySelection) []string {
	var warnings []string

	if stats.TotalChunks == 0 {
		warnings = append(warnings, "document produced no chunks")
		return warnings
	}

	if float64(stats.OrphanChunks)/float64(stats.TotalChunks) > 0.5 {
		warnings = append(warnings, "more than half of chunks have no section context")
	}

	if stats.MaxChunkTokens > selection.RecommendedSize*2 {
		warnings = append(warnings, "some chunks are much larger than the target size")
	}

	if profile.TableLikeLines > 0 && selection.Strategy != StrategyHeuristic {
		warnings = append(warnings, "document contains table-like lines; heuristic strategy may produce better results")
	}

	if profile.CodeBlocks > 3 && selection.Strategy != StrategyHeuristic {
		warnings = append(warnings, "document contains many code blocks; heuristic strategy may preserve them better")
	}

	return warnings
}

func splitByHeading(text string, size, overlap int) []chunker.Chunk {
	recursive := chunker.Recursive{SizeTokens: size, OverlapTokens: overlap}
	return recursive.Split(text)
}

func splitHeuristic(text string, size, overlap int) []chunker.Chunk {
	recursive := chunker.Recursive{SizeTokens: size, OverlapTokens: overlap}
	return recursive.Split(text)
}
