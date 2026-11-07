package git

// Trace HEAD's first-parent history through Git's graph geometry. This keeps
// the branch identity independent of lane numbers and Git's changing palette.
func markCurrentBranch(rows []GraphLine, chain []string) {
	type point struct{ row, col int }
	nodes := map[string]point{}
	for r, row := range rows {
		if row.Hash == "" {
			continue
		}
		for c, cell := range row.Cells {
			if cell.Glyph == '●' {
				nodes[row.Hash] = point{r, c}
				break
			}
		}
	}
	for i, hash := range chain {
		start, ok := nodes[hash]
		if !ok {
			continue
		}
		rows[start.row].CurrentBranch = true
		rows[start.row].Cells[start.col].CurrentBranch = true
		if i+1 >= len(chain) {
			continue
		}
		end, ok := nodes[chain[i+1]]
		if !ok || end.row <= start.row {
			continue
		}
		queue := []point{start}
		previous := map[point]point{start: start}
		found := false
		add := func(from, to point) {
			if to.row < start.row || to.row > end.row || to.col < 0 || to.col >= len(rows[to.row].Cells) {
				return
			}
			glyph := rows[to.row].Cells[to.col].Glyph
			if glyph == ' ' || glyph == '●' && to != start && to != end {
				return
			}
			if _, visited := previous[to]; visited {
				return
			}
			previous[to] = from
			queue = append(queue, to)
		}
		for head := 0; head < len(queue); head++ {
			current := queue[head]
			if current == end {
				found = true
				break
			}
			glyph := rows[current.row].Cells[current.col].Glyph
			if current.row < end.row {
				for _, delta := range []int{0, -1, 1} {
					col := current.col + delta
					if col < 0 || col >= len(rows[current.row+1].Cells) {
						continue
					}
					next := rows[current.row+1].Cells[col].Glyph
					outgoing := glyph == '●' || glyph == '─' || glyph == '│' && delta == 0 || glyph == '╱' && delta == -1 || glyph == '╲' && delta == 1
					incoming := next == '●' || next == '│' || next == '─' || next == '╱' && delta == -1 || next == '╲' && delta == 1
					if outgoing && incoming {
						add(current, point{current.row + 1, col})
					}
				}
			}
			// Git uses horizontal bridges for collapsed lanes and octopus merges.
			for _, delta := range []int{-1, 1} {
				col := current.col + delta
				if col >= 0 && col < len(rows[current.row].Cells) && (glyph == '─' || rows[current.row].Cells[col].Glyph == '─') {
					add(current, point{current.row, col})
				}
			}
		}
		if found {
			for current := end; ; current = previous[current] {
				rows[current.row].Cells[current.col].CurrentBranch = true
				if current == start {
					break
				}
			}
		}
	}
}
