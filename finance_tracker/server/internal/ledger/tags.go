package ledger

import (
	"sort"
	"strings"
)

// Tags are stored as ",a,b," so a single LIKE '%,a,%' finds them. Tags are
// for who/why/where (kids, kristina, trip:rome, house) — not for what kind of
// spending it was, which is the category's job, nor who was paid, which is
// the merchant's.

// NormalizeTags lower-cases, trims, de-duplicates and sorts a tag list.
func NormalizeTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		t = strings.Trim(t, ",")
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// SplitTags parses either storage form (",a,b,") or user input ("a, b").
func SplitTags(s string) []string {
	return NormalizeTags(strings.Split(s, ","))
}

// JoinTags renders the storage form.
func JoinTags(tags []string) string {
	tags = NormalizeTags(tags)
	if len(tags) == 0 {
		return ""
	}
	return "," + strings.Join(tags, ",") + ","
}

// HasTag reports whether a storage-form tag string carries t.
func HasTag(stored, t string) bool {
	return strings.Contains(stored, ","+strings.ToLower(t)+",")
}

// TripTag reports whether a tag names a trip.
func TripTag(t string) bool { return strings.HasPrefix(t, "trip:") }

// MergeSuggestion proposes folding one tag into another.
type MergeSuggestion struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// SuggestTagMerges finds likely duplicates among tags in use: a plural and
// its singular, near-identical spellings, and separator variants. The
// rarer tag is proposed to fold into the more used one. Suggestions only.
func SuggestTagMerges(tags []TagCount) []MergeSuggestion {
	count := map[string]int{}
	for _, t := range tags {
		count[t.Tag] = t.Count
	}
	norm := func(s string) string { return strings.NewReplacer("-", "", "_", "", " ", "", ":", "").Replace(s) }
	seen := map[string]bool{}
	var out []MergeSuggestion
	add := func(a, b, reason string) {
		if a == b || TripTag(a) != TripTag(b) {
			return
		}
		from, to := a, b
		if count[a] > count[b] || (count[a] == count[b] && a < b) {
			from, to = b, a
		}
		if seen[from+">"+to] {
			return
		}
		seen[from+">"+to] = true
		out = append(out, MergeSuggestion{From: from, To: to, Reason: reason})
	}
	for i, a := range tags {
		for _, b := range tags[i+1:] {
			x, y := a.Tag, b.Tag
			switch {
			case x+"s" == y || y+"s" == x || x+"es" == y || y+"es" == x:
				add(x, y, "singular and plural")
			case norm(x) == norm(y):
				add(x, y, "same word, different separator")
			case len(x) >= 5 && len(y) >= 5 && editDistance(x, y) == 1:
				add(x, y, "one letter apart")
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
