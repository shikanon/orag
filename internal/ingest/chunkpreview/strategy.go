package chunkpreview

const (
	StrategyAuto      = "auto"
	StrategyHeading   = "heading"
	StrategyRecursive = "recursive"
	StrategyHeuristic = "heuristic"
)

// StrategySelection describes which chunking strategy was chosen, why, and with
// what confidence. It also carries recommended size/overlap values tuned to the
// document profile.
type StrategySelection struct {
	Strategy           string
	Reason             string
	Confidence         float64
	RecommendedSize    int
	RecommendedOverlap int
}

// SelectStrategy picks a chunking strategy based on the document profile and
// user configuration. When config.Strategy is not "auto", the configured
// strategy is returned unchanged along with a confidence score and reason.
func SelectStrategy(profile DocumentProfile, config Config) StrategySelection {
	size := config.SizeTokens
	if size <= 0 {
		size = 800
	}
	overlap := config.OverlapTokens
	if overlap <= 0 {
		overlap = 100
	}

	switch config.Strategy {
	case StrategyHeading:
		return StrategySelection{
			Strategy:           StrategyHeading,
			Reason:             "heading strategy explicitly requested",
			Confidence:         1.0,
			RecommendedSize:    size,
			RecommendedOverlap: overlap,
		}
	case StrategyRecursive:
		return StrategySelection{
			Strategy:           StrategyRecursive,
			Reason:             "recursive strategy explicitly requested",
			Confidence:         1.0,
			RecommendedSize:    size,
			RecommendedOverlap: overlap,
		}
	case StrategyHeuristic:
		return StrategySelection{
			Strategy:           StrategyHeuristic,
			Reason:             "heuristic strategy explicitly requested",
			Confidence:         1.0,
			RecommendedSize:    size,
			RecommendedOverlap: overlap,
		}
	}

	return selectAutoStrategy(profile, size, overlap)
}

func selectAutoStrategy(profile DocumentProfile, size, overlap int) StrategySelection {
	headingRatio := 0.0
	if profile.ParagraphCount > 0 {
		headingRatio = float64(profile.HeadingCount) / float64(profile.ParagraphCount)
	}

	tableAndCodeDensity := 0.0
	if profile.ParagraphCount > 0 {
		tableAndCodeDensity = float64(profile.TableLikeLines+profile.CodeBlocks*5) / float64(profile.ParagraphCount)
	}

	longParagraphRatio := 0.0
	if profile.ParagraphCount > 0 {
		longParagraphRatio = float64(profile.LongParagraphs) / float64(profile.ParagraphCount)
	}

	if profile.TotalTokens < 2000 {
		return StrategySelection{
			Strategy:           StrategyRecursive,
			Reason:             "short document (< 2000 tokens); recursive chunking works well with no tuning needed",
			Confidence:         0.85,
			RecommendedSize:    min(size, 400),
			RecommendedOverlap: min(overlap, 50),
		}
	}

	if profile.HeadingCount > 10 && headingRatio > 0.15 {
		return StrategySelection{
			Strategy:           StrategyHeading,
			Reason:             "document has rich heading structure (> 10 headings, high heading/paragraph ratio); heading-based chunking preserves semantic boundaries",
			Confidence:         0.9,
			RecommendedSize:    size,
			RecommendedOverlap: overlap,
		}
	}

	if tableAndCodeDensity > 0.3 {
		return StrategySelection{
			Strategy:           StrategyHeuristic,
			Reason:             "document contains many tables and code blocks; heuristic chunking handles structured content better",
			Confidence:         0.8,
			RecommendedSize:    size,
			RecommendedOverlap: overlap,
		}
	}

	if longParagraphRatio > 0.3 {
		return StrategySelection{
			Strategy:           StrategyRecursive,
			Reason:             "document has many long paragraphs; recursive chunking splits them evenly with overlap",
			Confidence:         0.82,
			RecommendedSize:    size,
			RecommendedOverlap: overlap,
		}
	}

	return StrategySelection{
		Strategy:           StrategyRecursive,
		Reason:             "default choice for general-purpose documents with balanced structure",
		Confidence:         0.75,
		RecommendedSize:    size,
		RecommendedOverlap: overlap,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
