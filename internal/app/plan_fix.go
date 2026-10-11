package app

import (
	"fmt"
	"go/format"
	"regexp"
	"strings"
)

var reExcerptLine = regexp.MustCompile(`^\*\*Spec excerpt:\*\*[ \t]*(.*)$`)

// fixPlan repairs the mechanical mistakes a model makes in a plan, so a lint round is not spent on them:
// it gofmts whole-file Go blocks, unwraps an excerpt line wrapped in backticks, and restores spec excerpts
// that differ from the spec only in markup (backticks, asterisks) or whitespace. It never changes words.
// notes say what changed, or what could not be fixed.
func fixPlan(md, spec string) (string, []string) {
	var notes []string
	lines := strings.Split(md, "\n")
	for i := 0; i < len(lines); i++ {
		if m := reFence.FindStringSubmatch(lines[i]); m != nil {
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(lines[j], "```") {
				j++
			}
			file, mode := blockFileMode(m[2])
			if m[1] == "go" && file != "" && (mode == "" || mode == "create") {
				src := strings.Join(lines[i+1:j], "\n") + "\n"
				got, err := format.Source([]byte(src))
				switch {
				case err != nil:
					notes = append(notes, fmt.Sprintf("%s does not parse, left as written: %v", file, err))
				case string(got) != src:
					repl := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
					lines = append(lines[:i+1], append(repl, lines[j:]...)...)
					j = i + 1 + len(repl)
					notes = append(notes, "gofmt: "+file)
				}
			}
			i = j
			continue
		}
		if m := reExcerptLine.FindStringSubmatch(lines[i]); m != nil && spec != "" {
			if fixed, what := fixExcerpt(strings.TrimSpace(m[1]), spec); len(what) > 0 {
				lines[i] = "**Spec excerpt:** " + fixed
				notes = append(notes, what...)
			}
		}
	}
	return strings.Join(lines, "\n"), notes
}

// blockFileMode reads file= and mode= from a fence info string.
func blockFileMode(info string) (file, mode string) {
	for _, a := range reAttr.FindAllStringSubmatch(info, -1) {
		switch a[1] {
		case "file":
			file = strings.Trim(a[2], `"`)
		case "mode":
			mode = strings.Trim(a[2], `"`)
		}
	}
	return file, mode
}

// fixExcerpt returns the repaired excerpt and what it changed; what is empty when nothing changed.
func fixExcerpt(ex, spec string) (string, []string) {
	var what []string
	if inner := strings.Trim(ex, "` "); inner != ex && strings.HasPrefix(inner, `"`) && strings.HasSuffix(inner, `"`) {
		ex = inner
		what = append(what, "excerpt: removed the backticks around the quoted fragments")
	}
	frags, ok := excerptFragments(ex)
	if !ok {
		return ex, what
	}
	specN := normSpace(spec)
	for k, f := range frags {
		if strings.Contains(f, "...") || strings.Contains(f, "…") || strings.Contains(specN, normSpace(f)) {
			continue
		}
		if exact := markupMatch(specN, normSpace(f)); exact != "" {
			what = append(what, fmt.Sprintf("excerpt: %q -> %q (spec markup restored)", f, exact))
			frags[k] = exact
		}
	}
	if len(what) == 0 {
		return ex, nil
	}
	return `"` + strings.Join(frags, `" and "`) + `"`, what
}

func isMarkup(b byte) bool { return b == '`' || b == '*' }

// stripMarkup drops backticks and asterisks; idx maps each kept byte back to its position in s.
func stripMarkup(s string) (string, []int) {
	var b strings.Builder
	var idx []int
	for i := 0; i < len(s); i++ {
		if !isMarkup(s[i]) {
			b.WriteByte(s[i])
			idx = append(idx, i)
		}
	}
	return b.String(), idx
}

// markupMatch finds frag in specN ignoring markup and returns the spec's own text for it, including any
// markup that opens or closes at the edges; "" when the words do not match.
func markupMatch(specN, frag string) string {
	loose, idx := stripMarkup(specN)
	lf, _ := stripMarkup(frag)
	if lf == "" {
		return ""
	}
	at := strings.Index(loose, lf)
	if at < 0 {
		return ""
	}
	start, end := idx[at], idx[at+len(lf)-1]+1
	for start > 0 && isMarkup(specN[start-1]) {
		start--
	}
	for end < len(specN) && isMarkup(specN[end]) {
		end++
	}
	return specN[start:end]
}

// closestSpecPhrase returns the run of spec words most like frag (case and markup ignored), when it is
// close enough to be a typo rather than a paraphrase. It backs the "did you mean" hint in plan lint.
func closestSpecPhrase(spec, frag string) (string, bool) {
	words := strings.Fields(spec)
	n := len(strings.Fields(frag))
	if n == 0 || len(words) == 0 {
		return "", false
	}
	key := func(s string) string { l, _ := stripMarkup(strings.ToLower(s)); return l }
	target := key(normSpace(frag))
	best, bestScore := "", 0.0
	for size := max(1, n-1); size <= n+1; size++ {
		for i := 0; i+size <= len(words); i++ {
			cand := strings.Join(words[i:i+size], " ")
			c := key(cand)
			longer := max(len(c), len(target))
			if longer == 0 {
				continue
			}
			if score := 1 - float64(levenshtein(c, target))/float64(longer); score > bestScore {
				best, bestScore = cand, score
			}
		}
	}
	return best, bestScore >= 0.8
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
