package topicplanning

import "strings"

// The line difference between a suggestion's base version and its proposed
// body (specs/036 PR 2; FR-035, contract §6).
//
// DiffLines is pure: it reads no storage and no clock, and the same two
// strings always give the same bytes. It is computed on every read rather
// than stored: versions are insert-only, so the base body a read sees is the
// body the suggestion was written against, and a stored copy could only ever
// drift from the proposed body it describes.

// DiffOpKind is one of the three things a line can be.
type DiffOpKind string

const (
	DiffEqual  DiffOpKind = "equal"
	DiffDelete DiffOpKind = "delete"
	DiffInsert DiffOpKind = "insert"
)

// DiffOp is a run of consecutive lines of one kind. Text is the lines joined
// with "\n", exactly as they are: no normalization, no trimming, a "\r" kept
// where it was. An op always holds at least one line.
type DiffOp struct {
	Op   DiffOpKind `json:"op"`
	Text string     `json:"text"`
}

// Diff is a shortest edit script from the base to the proposed body, by line.
type Diff struct {
	Ops           []DiffOp `json:"ops"`
	InsertedLines int      `json:"inserted_lines"`
	DeletedLines  int      `json:"deleted_lines"`
}

// splitDiffLines cuts a body at "\n". The empty body has no lines; any other
// body has one more line than it has "\n", so a trailing newline is a line of
// its own and a difference in it is a one-line difference.
func splitDiffLines(body string) []string {
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// DiffLines is the shortest line edit script from base to proposed (Myers,
// O((N+M)·D) time in linear space). Where several scripts are equally short,
// the one the algorithm finds is kept, and inside each changed stretch the
// deleted lines come before the inserted ones, so the answer is fixed by the
// input alone. Adjacent lines of one kind are merged into one op.
func DiffLines(base, proposed string) Diff {
	a, b := splitDiffLines(base), splitDiffLines(proposed)
	// Lines are compared as small integers: equal text, equal number.
	ids := map[string]int{}
	intern := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, line := range lines {
			id, ok := ids[line]
			if !ok {
				id = len(ids)
				ids[line] = id
			}
			out[i] = id
		}
		return out
	}
	d := &lineDiffer{a: intern(a), b: intern(b)}
	d.compare(0, len(a), 0, len(b))

	diff := Diff{Ops: []DiffOp{}}
	var equal, deleted, inserted []string
	flushEqual := func() {
		if len(equal) > 0 {
			diff.Ops = append(diff.Ops, DiffOp{Op: DiffEqual, Text: strings.Join(equal, "\n")})
			equal = nil
		}
	}
	flushChange := func() {
		if len(deleted) > 0 {
			diff.Ops = append(diff.Ops, DiffOp{Op: DiffDelete, Text: strings.Join(deleted, "\n")})
			diff.DeletedLines += len(deleted)
		}
		if len(inserted) > 0 {
			diff.Ops = append(diff.Ops, DiffOp{Op: DiffInsert, Text: strings.Join(inserted, "\n")})
			diff.InsertedLines += len(inserted)
		}
		deleted, inserted = nil, nil
	}
	for _, step := range d.steps {
		switch step.kind {
		case DiffEqual:
			flushChange()
			equal = append(equal, a[step.index])
		case DiffDelete:
			flushEqual()
			deleted = append(deleted, a[step.index])
		case DiffInsert:
			flushEqual()
			inserted = append(inserted, b[step.index])
		}
	}
	flushChange()
	flushEqual()
	return diff
}

type diffStep struct {
	kind  DiffOpKind
	index int // into a for equal and delete, into b for insert
}

type lineDiffer struct {
	a, b  []int
	steps []diffStep
}

// compare appends the steps turning a[a0:a1] into b[b0:b1].
func (d *lineDiffer) compare(a0, a1, b0, b1 int) {
	for a0 < a1 && b0 < b1 && d.a[a0] == d.b[b0] {
		d.steps = append(d.steps, diffStep{DiffEqual, a0})
		a0++
		b0++
	}
	suffix := 0
	for a1 > a0 && b1 > b0 && d.a[a1-1] == d.b[b1-1] {
		a1--
		b1--
		suffix++
	}
	switch {
	case a0 == a1:
		for j := b0; j < b1; j++ {
			d.steps = append(d.steps, diffStep{DiffInsert, j})
		}
	case b0 == b1:
		for i := a0; i < a1; i++ {
			d.steps = append(d.steps, diffStep{DiffDelete, i})
		}
	default:
		if x, y, ok := d.bisect(a0, a1, b0, b1); ok {
			d.compare(a0, x, b0, y)
			d.compare(x, a1, y, b1)
		} else {
			// Nothing in common: every line goes, every line comes.
			for i := a0; i < a1; i++ {
				d.steps = append(d.steps, diffStep{DiffDelete, i})
			}
			for j := b0; j < b1; j++ {
				d.steps = append(d.steps, diffStep{DiffInsert, j})
			}
		}
	}
	for i := range suffix {
		d.steps = append(d.steps, diffStep{DiffEqual, a1 + i})
	}
}

// bisect finds a point (x, y) on a shortest edit path through a[a0:a1] and
// b[b0:b1], where the forward and the reverse search meet: Myers' middle
// snake, in the formulation diff-match-patch uses. ok is false when the two
// ranges share no line at all.
func (d *lineDiffer) bisect(a0, a1, b0, b1 int) (int, int, bool) {
	n, m := a1-a0, b1-b0
	maxD := (n + m + 1) / 2
	offset := maxD
	length := 2*maxD + 2
	v1 := make([]int, length)
	v2 := make([]int, length)
	for i := range v1 {
		v1[i], v2[i] = -1, -1
	}
	v1[offset+1], v2[offset+1] = 0, 0
	delta := n - m
	front := delta%2 != 0
	k1start, k1end, k2start, k2end := 0, 0, 0, 0
	for step := 0; step < maxD; step++ {
		for k1 := -step + k1start; k1 <= step-k1end; k1 += 2 {
			k1off := offset + k1
			var x1 int
			if k1 == -step || (k1 != step && v1[k1off-1] < v1[k1off+1]) {
				x1 = v1[k1off+1]
			} else {
				x1 = v1[k1off-1] + 1
			}
			y1 := x1 - k1
			for x1 < n && y1 < m && d.a[a0+x1] == d.b[b0+y1] {
				x1++
				y1++
			}
			v1[k1off] = x1
			switch {
			case x1 > n:
				k1end += 2
			case y1 > m:
				k1start += 2
			case front:
				k2off := offset + delta - k1
				if k2off >= 0 && k2off < length && v2[k2off] != -1 && x1 >= n-v2[k2off] {
					return a0 + x1, b0 + y1, true
				}
			}
		}
		for k2 := -step + k2start; k2 <= step-k2end; k2 += 2 {
			k2off := offset + k2
			var x2 int
			if k2 == -step || (k2 != step && v2[k2off-1] < v2[k2off+1]) {
				x2 = v2[k2off+1]
			} else {
				x2 = v2[k2off-1] + 1
			}
			y2 := x2 - k2
			for x2 < n && y2 < m && d.a[a1-x2-1] == d.b[b1-y2-1] {
				x2++
				y2++
			}
			v2[k2off] = x2
			switch {
			case x2 > n:
				k2end += 2
			case y2 > m:
				k2start += 2
			case !front:
				k1off := offset + delta - k2
				if k1off >= 0 && k1off < length && v1[k1off] != -1 {
					x1 := v1[k1off]
					y1 := offset + x1 - k1off
					if x1 >= n-x2 {
						return a0 + x1, b0 + y1, true
					}
				}
			}
		}
	}
	return 0, 0, false
}
