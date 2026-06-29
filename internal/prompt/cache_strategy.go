package prompt

import (
	"sort"
	"strings"
)

type CacheMode string

const (
	CacheModeAuto   CacheMode = "auto"
	CacheModeManual CacheMode = "manual"
	CacheModeNone   CacheMode = "none"
)

type Segment struct {
	Name    string
	Stable  bool
	Content string
}

type CacheStrategy interface {
	Mode() CacheMode
	Apply([]Segment) string
}

type Strategy struct {
	mode CacheMode
}

func NewStrategy(mode string) Strategy {
	switch CacheMode(strings.ToLower(strings.TrimSpace(mode))) {
	case CacheModeManual:
		return Strategy{mode: CacheModeManual}
	case CacheModeNone:
		return Strategy{mode: CacheModeNone}
	default:
		return Strategy{mode: CacheModeAuto}
	}
}

func (s Strategy) Mode() CacheMode {
	return s.mode
}

func (s Strategy) Apply(segments []Segment) string {
	cp := append([]Segment(nil), segments...)
	sort.SliceStable(cp, func(i, j int) bool {
		if cp[i].Stable == cp[j].Stable {
			return cp[i].Name < cp[j].Name
		}
		return cp[i].Stable
	})
	var b strings.Builder
	for _, seg := range cp {
		if seg.Content == "" {
			continue
		}
		if s.mode == CacheModeManual && seg.Stable {
			b.WriteString("<cache_control ttl=\"3600\">\n")
			b.WriteString(seg.Content)
			b.WriteString("\n</cache_control>\n")
			continue
		}
		b.WriteString(seg.Content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}
