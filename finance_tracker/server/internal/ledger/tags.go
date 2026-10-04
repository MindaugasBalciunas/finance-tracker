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
