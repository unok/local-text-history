package diff

import (
	"strings"
	"testing"
)

func TestUnifiedDiff_BasicChange(t *testing.T) {
	from := "line1\nline2\nline3\n"
	to := "line1\nmodified\nline3\n"

	result := UnifiedDiff(from, to, "a/file.go", "b/file.go")

	if !strings.Contains(result, "--- a/file.go") {
		t.Error("missing from label")
	}
	if !strings.Contains(result, "+++ b/file.go") {
		t.Error("missing to label")
	}
	if !strings.Contains(result, "-line2") {
		t.Error("missing deleted line")
	}
	if !strings.Contains(result, "+modified") {
		t.Error("missing added line")
	}
	if !strings.Contains(result, "@@") {
		t.Error("missing hunk header")
	}
}

func TestUnifiedDiff_NoChanges(t *testing.T) {
	text := "line1\nline2\nline3\n"

	result := UnifiedDiff(text, text, "a/file.go", "b/file.go")

	if result != "" {
		t.Errorf("expected empty diff, got:\n%s", result)
	}
}

func TestUnifiedDiff_Addition(t *testing.T) {
	from := "line1\nline2\n"
	to := "line1\nline2\nline3\n"

	result := UnifiedDiff(from, to, "a/file.go", "b/file.go")

	if !strings.Contains(result, "+line3") {
		t.Errorf("missing added line, got:\n%s", result)
	}
}

func TestUnifiedDiff_Deletion(t *testing.T) {
	from := "line1\nline2\nline3\n"
	to := "line1\nline3\n"

	result := UnifiedDiff(from, to, "a/file.go", "b/file.go")

	if !strings.Contains(result, "-line2") {
		t.Errorf("missing deleted line, got:\n%s", result)
	}
}

func TestUnifiedDiff_EmptyFrom(t *testing.T) {
	from := ""
	to := "new content\n"

	result := UnifiedDiff(from, to, "a/file.go", "b/file.go")

	if !strings.Contains(result, "+new content") {
		t.Errorf("missing added content, got:\n%s", result)
	}
}

func TestUnifiedDiff_EmptyTo(t *testing.T) {
	from := "old content\n"
	to := ""

	result := UnifiedDiff(from, to, "a/file.go", "b/file.go")

	if !strings.Contains(result, "-old content") {
		t.Errorf("missing deleted content, got:\n%s", result)
	}
}

func TestUnifiedDiff_MultipleHunks(t *testing.T) {
	var fromLines, toLines []string
	for i := 1; i <= 20; i++ {
		line := "line" + strings.Repeat(" ", i)
		fromLines = append(fromLines, line)
		if i == 3 {
			toLines = append(toLines, "changed3")
		} else if i == 17 {
			toLines = append(toLines, "changed17")
		} else {
			toLines = append(toLines, line)
		}
	}

	from := strings.Join(fromLines, "\n") + "\n"
	to := strings.Join(toLines, "\n") + "\n"

	result := UnifiedDiff(from, to, "a/file.go", "b/file.go")

	// Should have two separate hunks
	hunkCount := strings.Count(result, "@@")
	if hunkCount < 2 {
		t.Errorf("expected at least 2 hunk headers, got %d:\n%s", hunkCount, result)
	}
}
