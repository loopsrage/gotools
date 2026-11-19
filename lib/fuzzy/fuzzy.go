package fuzzy

import (
	"sort"
	"strings"
)

// Match represents a candidate match and its similarity score in [0, 100].
type Match struct {
	Choice string  `json:"choice"`
	Score  float64 `json:"score"`
	Index  int     `json:"index"`
}

// IndelDistance computes the minimum number of insertions + deletions (LCS-based)
// needed to transform rune slice a into b (matching rapidfuzz.fuzz.ratio).
func IndelDistance(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	na, nb := len(ra), len(rb)
	if na == 0 {
		return nb
	}
	if nb == 0 {
		return na
	}
	prev := make([]int, nb+1)
	curr := make([]int, nb+1)
	for i := 1; i <= na; i++ {
		for j := 1; j <= nb; j++ {
			if ra[i-1] == rb[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] >= curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
	}
	lcs := prev[nb]
	return (na - lcs) + (nb - lcs)
}

// Ratio returns the normalized Indel similarity in [0, 100], identical to rapidfuzz.fuzz.ratio.
func Ratio(a, b string) float64 {
	a = strings.TrimSpace(strings.ToLower(a))
	b = strings.TrimSpace(strings.ToLower(b))
	total := len([]rune(a)) + len([]rune(b))
	if total == 0 {
		return 100.0
	}
	dist := IndelDistance(a, b)
	return (1.0 - float64(dist)/float64(total)) * 100.0
}

// FuzzOneAgainstChoices scores query against every candidate in choices, filtering by minScore
// and returning matches sorted by score descending.
func FuzzOneAgainstChoices(query string, choices []string, minScore float64, limit int) []Match {
	matches := make([]Match, 0, len(choices))
	for i, c := range choices {
		sc := Ratio(query, c)
		if sc >= minScore {
			matches = append(matches, Match{Choice: c, Score: sc, Index: i})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Index < matches[j].Index
		}
		return matches[i].Score > matches[j].Score
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// BestMatch returns the highest-scoring choice >= minScore, or ok=false if none meets the threshold.
func BestMatch(query string, choices []string, minScore float64) (Match, bool) {
	m := FuzzOneAgainstChoices(query, choices, minScore, 1)
	if len(m) == 0 {
		return Match{}, false
	}
	return m[0], true
}
