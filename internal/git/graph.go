package git

import (
	"strconv"
	"strings"
)

// Git computes the topology and lane colors; only its graph prefix is decoded.
// Subjects and refs remain data and are sanitized by the renderer.
func readGraphs(s *Status, run func(...string) (string, error)) {
	s.CurrentGraphLines = []GraphLine{}
	s.Head, _ = run("rev-parse", "--verify", "HEAD")
	refs := map[string][]GraphRef{}
	branches := map[string]string{}
	raw, _ := run("for-each-ref", "--format=%(objectname)%00%(refname)%00%(symref)%00%(*objectname)", "refs/heads/", "refs/remotes/", "refs/tags/")
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 || fields[2] != "" {
			continue
		}
		hash, name := fields[0], fields[1]
		if fields[3] != "" {
			hash = fields[3]
		}
		ref := GraphRef{}
		switch {
		case strings.HasPrefix(name, "refs/heads/"):
			ref.Kind, ref.Name = "local", strings.TrimPrefix(name, "refs/heads/")
			ref.Current = ref.Name == s.Branch
		case strings.HasPrefix(name, "refs/remotes/"):
			ref.Kind, ref.Name = "remote", strings.TrimPrefix(name, "refs/remotes/")
		case strings.HasPrefix(name, "refs/tags/"):
			ref.Kind, ref.Name = "tag", strings.TrimPrefix(name, "refs/tags/")
		default:
			continue
		}
		refs[hash] = append(refs[hash], ref)
		branches[name] = hash
	}
	upstream, upstreamErr := run("rev-parse", "--symbolic-full-name", "@{upstream}")
	if upstreamErr != nil {
		upstream = ""
	}
	if upstream == "" {
		candidate := "refs/remotes/origin/" + s.Branch
		if branches[candidate] != "" {
			upstream = candidate
		}
	}
	s.upstreamRef = upstream
	s.Upstream = strings.TrimPrefix(upstream, "refs/remotes/")
	if s.Head == "" {
		return
	}
	read := func(revisions ...string) []GraphLine {
		args := []string{"-c", "log.graphColors=cyan,magenta,green,yellow,blue,red", "log", "--graph", "--topo-order", "--color=always", "--format=%x00%H%x00%h%x00%s"}
		args = append(args, revisions...)
		args = append(args, "--")
		graph, err := run(args...)
		if err != nil {
			return []GraphLine{}
		}
		return parseGraph(graph, refs, s.Head)
	}
	ancestry, _ := run("rev-list", "--first-parent", "HEAD")
	chain := strings.Fields(ancestry)
	s.GraphLines = read("--branches", "--remotes", "HEAD")
	markCurrentBranch(s.GraphLines, chain)
	revisions := []string{"HEAD"}
	if upstream != "" {
		revisions = append(revisions, upstream)
	}
	s.CurrentGraphLines = read(revisions...)
	markCurrentBranch(s.CurrentGraphLines, chain)
	for i := range s.CurrentGraphLines {
		visible := []GraphRef{}
		for _, ref := range s.CurrentGraphLines[i].Refs {
			if ref.Current || ref.Kind == "remote" && ref.Name == s.Upstream || ref.Kind == "tag" {
				visible = append(visible, ref)
			}
		}
		s.CurrentGraphLines[i].Refs = visible
	}
	texts := []string{}
	for _, row := range s.GraphLines {
		texts = append(texts, row.Text)
	}
	s.Graph = strings.Join(texts, "\n")
}

func parseGraph(raw string, refs map[string][]GraphRef, head string) []GraphLine {
	rows := []GraphLine{}
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\x00", 4)
		row := GraphLine{Cells: graphCells(fields[0])}
		// Git leaves commit dots uncolored even when their lane is colored.
		for i := range row.Cells {
			if row.Cells[i].Glyph == '●' && row.Cells[i].Color < 0 {
				row.Cells[i].Color = (i / 2) % 6
			}
		}
		prefix := ""
		for _, cell := range row.Cells {
			prefix += string(cell.Glyph)
		}
		row.Text = prefix
		if len(fields) == 4 {
			row.Hash, row.ShortHash, row.Subject = fields[1], fields[2], fields[3]
			row.Refs, row.Current = refs[row.Hash], row.Hash == head
			row.Text += row.ShortHash + " " + row.Subject
		}
		rows = append(rows, row)
	}
	return rows
}

func graphCells(prefix string) []GraphCell {
	cells := []GraphCell{}
	color := -1
	for i := 0; i < len(prefix); {
		if prefix[i] == 27 && i+1 < len(prefix) && prefix[i+1] == '[' {
			end := strings.IndexByte(prefix[i+2:], 'm')
			if end < 0 {
				break
			}
			code := prefix[i+2 : i+2+end]
			color = -1
			for _, part := range strings.Split(code, ";") {
				n, _ := strconv.Atoi(part)
				switch n {
				case 36:
					color = 0
				case 35:
					color = 1
				case 32:
					color = 2
				case 33:
					color = 3
				case 34:
					color = 4
				case 31:
					color = 5
				}
			}
			i += end + 3
			continue
		}
		glyph := rune(prefix[i])
		i++
		switch glyph {
		case '*':
			glyph = '●'
		case '|':
			glyph = '│'
		case '/':
			glyph = '╱'
		case '\\':
			glyph = '╲'
		case '_', '-':
			glyph = '─'
		}
		cells = append(cells, GraphCell{Glyph: glyph, Color: color})
	}
	return cells
}
