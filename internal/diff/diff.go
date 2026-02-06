package diff

import (
	"fmt"
	"strings"

	difflib "github.com/sergi/go-diff/diffmatchpatch"
)

// UnifiedDiff generates a unified diff between two texts.
func UnifiedDiff(fromText, toText, fromLabel, toLabel string) string {
	dmp := difflib.New()
	a, b, c := dmp.DiffLinesToChars(fromText, toText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, c)
	diffs = dmp.DiffCleanupSemantic(diffs)

	return formatUnifiedDiff(diffs, fromLabel, toLabel)
}

func formatUnifiedDiff(diffs []difflib.Diff, fromLabel, toLabel string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", fromLabel))
	sb.WriteString(fmt.Sprintf("+++ %s\n", toLabel))

	// Convert diffs to lines with context
	type line struct {
		op   difflib.Operation
		text string
	}

	var lines []line
	for _, d := range diffs {
		text := d.Text
		// Ensure text ends with newline for consistent splitting
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		for _, l := range strings.SplitAfter(text, "\n") {
			if l == "" {
				continue
			}
			lines = append(lines, line{op: d.Type, text: l})
		}
	}

	if len(lines) == 0 {
		return ""
	}

	const contextLines = 3

	// Find hunks: groups of changes with surrounding context
	type hunk struct {
		startFrom int
		startTo   int
		lines     []line
	}

	// Identify change regions
	type changeRegion struct {
		start, end int // indices into lines
	}
	var regions []changeRegion
	inChange := false
	var regionStart int
	for i, l := range lines {
		if l.op != difflib.DiffEqual {
			if !inChange {
				inChange = true
				regionStart = i
			}
		} else {
			if inChange {
				regions = append(regions, changeRegion{start: regionStart, end: i})
				inChange = false
			}
		}
	}
	if inChange {
		regions = append(regions, changeRegion{start: regionStart, end: len(lines)})
	}

	if len(regions) == 0 {
		return ""
	}

	// Merge overlapping/adjacent regions with context
	type expandedRegion struct {
		start, end int
	}
	var expanded []expandedRegion
	for _, r := range regions {
		start := r.start - contextLines
		if start < 0 {
			start = 0
		}
		end := r.end + contextLines
		if end > len(lines) {
			end = len(lines)
		}
		if len(expanded) > 0 && start <= expanded[len(expanded)-1].end {
			expanded[len(expanded)-1].end = end
		} else {
			expanded = append(expanded, expandedRegion{start: start, end: end})
		}
	}

	// Output hunks
	for _, er := range expanded {
		fromLine := 1
		toLine := 1
		for i := 0; i < er.start; i++ {
			switch lines[i].op {
			case difflib.DiffEqual:
				fromLine++
				toLine++
			case difflib.DiffDelete:
				fromLine++
			case difflib.DiffInsert:
				toLine++
			}
		}

		fromCount := 0
		toCount := 0
		for i := er.start; i < er.end; i++ {
			switch lines[i].op {
			case difflib.DiffEqual:
				fromCount++
				toCount++
			case difflib.DiffDelete:
				fromCount++
			case difflib.DiffInsert:
				toCount++
			}
		}

		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", fromLine, fromCount, toLine, toCount))

		for i := er.start; i < er.end; i++ {
			l := lines[i]
			text := strings.TrimSuffix(l.text, "\n")
			switch l.op {
			case difflib.DiffEqual:
				sb.WriteString(" " + text + "\n")
			case difflib.DiffDelete:
				sb.WriteString("-" + text + "\n")
			case difflib.DiffInsert:
				sb.WriteString("+" + text + "\n")
			}
		}
	}

	return sb.String()
}
