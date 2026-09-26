package topicplanning

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"
)

// specs/036 PR 2, no database: DiffLines (T036; FR-035, contract §6). The
// eight fixed samples carry the names contract §6 gives them.

func TestDiffLinesFixedSamples(t *testing.T) {
	for _, tc := range []struct {
		name              string
		base, proposed    string
		ops               []DiffOp
		inserted, deleted int
	}{
		{"identical", "第一行\n第二行", "第一行\n第二行",
			[]DiffOp{{DiffEqual, "第一行\n第二行"}}, 0, 0},
		{"append-line", "a\nb", "a\nb\nc",
			[]DiffOp{{DiffEqual, "a\nb"}, {DiffInsert, "c"}}, 1, 0},
		{"replace-first-line", "a\nb\nc", "x\nb\nc",
			[]DiffOp{{DiffDelete, "a"}, {DiffInsert, "x"}, {DiffEqual, "b\nc"}}, 1, 1},
		{"delete-middle", "a\nb\nc", "a\nc",
			[]DiffOp{{DiffEqual, "a"}, {DiffDelete, "b"}, {DiffEqual, "c"}}, 0, 1},
		{"trailing-newline", "a\nb", "a\nb\n",
			[]DiffOp{{DiffEqual, "a\nb"}, {DiffInsert, ""}}, 1, 0},
		{"empty-base", "", "a\nb",
			[]DiffOp{{DiffInsert, "a\nb"}}, 2, 0},
		{"chinese-paragraphs",
			"羊绒大衣不建议机洗。\n手洗时水温不超过 30 度，不要揉搓。\n平铺晾干，不要挂着晒。",
			"羊绒大衣不建议机洗。\n能机洗吗？只能用洗衣机的羊毛程序，水温不超过 30 度。\n平铺晾干，不要挂着晒。",
			[]DiffOp{
				{DiffEqual, "羊绒大衣不建议机洗。"},
				{DiffDelete, "手洗时水温不超过 30 度，不要揉搓。"},
				{DiffInsert, "能机洗吗？只能用洗衣机的羊毛程序，水温不超过 30 度。"},
				{DiffEqual, "平铺晾干，不要挂着晒。"},
			}, 1, 1},
		{"crlf-kept", "a\r\nb\r\n", "a\r\nc\r\n",
			[]DiffOp{{DiffEqual, "a\r"}, {DiffDelete, "b\r"}, {DiffInsert, "c\r"}, {DiffEqual, ""}}, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DiffLines(tc.base, tc.proposed)
			if !reflect.DeepEqual(got.Ops, tc.ops) || got.InsertedLines != tc.inserted || got.DeletedLines != tc.deleted {
				t.Fatalf("DiffLines = %+v, want ops %+v, +%d -%d", got, tc.ops, tc.inserted, tc.deleted)
			}
			if applied := applyDiff(got, false); applied != tc.proposed {
				t.Fatalf("applying the diff gives %q, want %q", applied, tc.proposed)
			}
		})
	}
	// Both empty: no lines, no ops - and an empty list, not null, on the wire.
	if encoded, _ := json.Marshal(DiffLines("", "")); string(encoded) != `{"ops":[],"inserted_lines":0,"deleted_lines":0}` {
		t.Fatalf("empty diff = %s", encoded)
	}
}

// applyDiff rebuilds one side from the ops: the proposed body from equal and
// insert, or the base from equal and delete.
func applyDiff(diff Diff, base bool) string {
	lines := []string{}
	for _, op := range diff.Ops {
		keep := op.Op == DiffEqual || (base && op.Op == DiffDelete) || (!base && op.Op == DiffInsert)
		if keep {
			lines = append(lines, strings.Split(op.Text, "\n")...)
		}
	}
	return strings.Join(lines, "\n")
}

// shortestEdit is the textbook dynamic-programming edit distance with
// insertions and deletions only: N + M - 2·LCS.
func shortestEdit(base, proposed string) int {
	a, b := splitDiffLines(base), splitDiffLines(proposed)
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	return len(a) + len(b) - 2*lcs[0][0]
}

// T036 property: 200 random pairs from a fixed seed. Applying the diff gives
// the proposed body back (and the base, read the other way); the script is as
// short as the dynamic-programming answer; no two neighbouring ops share a
// kind; an insert is never followed directly by a delete; and the same input
// gives the same bytes twice.
func TestDiffLinesProperties(t *testing.T) {
	random := rand.New(rand.NewPCG(20260925, 36))
	vocabulary := []string{"", "羊绒", "大衣", "机洗", "a", "b", "c\r", " ", "水温 30 度"}
	text := func() string {
		if random.IntN(8) == 0 {
			return ""
		}
		lines := make([]string, random.IntN(14))
		for i := range lines {
			lines[i] = vocabulary[random.IntN(len(vocabulary))]
		}
		body := strings.Join(lines, "\n")
		if random.IntN(3) == 0 {
			body += "\n"
		}
		return body
	}
	for i := range 200 {
		base, proposed := text(), text()
		diff := DiffLines(base, proposed)
		if got := applyDiff(diff, false); got != proposed {
			t.Fatalf("pair %d: applying gives %q, want %q (base %q)", i, got, proposed, base)
		}
		if got := applyDiff(diff, true); got != base {
			t.Fatalf("pair %d: reading back the base gives %q, want %q", i, got, base)
		}
		if want := shortestEdit(base, proposed); diff.InsertedLines+diff.DeletedLines != want {
			t.Fatalf("pair %d: %d edits, the shortest is %d (%q -> %q)", i, diff.InsertedLines+diff.DeletedLines, want, base, proposed)
		}
		for j := 1; j < len(diff.Ops); j++ {
			previous, current := diff.Ops[j-1].Op, diff.Ops[j].Op
			if previous == current {
				t.Fatalf("pair %d: two neighbouring %s ops: %+v", i, current, diff.Ops)
			}
			if previous == DiffInsert && current == DiffDelete {
				t.Fatalf("pair %d: an insert before a delete: %+v", i, diff.Ops)
			}
		}
		first, _ := json.Marshal(diff)
		second, _ := json.Marshal(DiffLines(base, proposed))
		if string(first) != string(second) {
			t.Fatalf("pair %d: two runs differ:\n%s\n%s", i, first, second)
		}
	}
}

// A whole body of the largest size, rewritten completely, stays fast and
// small: linear space, not a table of N·M.
func TestDiffLinesOnALargeRewrite(t *testing.T) {
	base, proposed := make([]string, 4000), make([]string, 4000)
	for i := range base {
		base[i] = "旧的第" + strings.Repeat("一", i%7) + "行"
		proposed[i] = "新的第" + strings.Repeat("二", i%5) + "行"
	}
	started := time.Now()
	diff := DiffLines(strings.Join(base, "\n"), strings.Join(proposed, "\n"))
	if diff.DeletedLines != 4000 || diff.InsertedLines != 4000 {
		t.Fatalf("a complete rewrite = +%d -%d", diff.InsertedLines, diff.DeletedLines)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("a complete rewrite of 4000 lines took %s", elapsed)
	}
}
