package FileToolInMainTool

import (
	"bytes"
	"strings"
	"unicode/utf8"

	agenttool "github.com/caliniya/caf/core/Agent/AgentTool"
)

func fileRecoveryChanges(ctx *agenttool.ToolContext, toolID, recoveryPath string) ([]agenttool.ToolChange, error) {
	record, _, err := readFileRecovery(ctx, toolID, recoveryPath)
	if err != nil {
		return nil, err
	}
	change := agenttool.ToolChange{
		Target: record.Path, Action: "modify",
		BeforeBytes: len(record.Before.Data), AfterBytes: len(record.After.Data),
	}
	if !record.Before.Exists {
		change.Action = "create"
	} else if !record.After.Exists {
		change.Action = "delete"
	}
	change.Binary = !utf8.Valid(record.Before.Data) || !utf8.Valid(record.After.Data) ||
		bytes.IndexByte(record.Before.Data, 0) >= 0 || bytes.IndexByte(record.After.Data, 0) >= 0
	if !change.Binary {
		change.AddedLines, change.RemovedLines, change.Approximate = changedFileLines(record.Before.Data, record.After.Data)
	}
	return []agenttool.ToolChange{change}, nil
}

func fileChangeLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Myers 最短编辑路径统计真正的增删行，保留未变的中间行和换行符。
// 大量变化时限制比较次数，回退到去掉公共首尾后的替换范围并明确标为估算。
func changedFileLines(before, after []byte) (added, removed int, approximate bool) {
	old, next := fileChangeLines(before), fileChangeLines(after)
	for len(old) > 0 && len(next) > 0 && old[0] == next[0] {
		old, next = old[1:], next[1:]
	}
	for len(old) > 0 && len(next) > 0 && old[len(old)-1] == next[len(next)-1] {
		old, next = old[:len(old)-1], next[:len(next)-1]
	}
	n, m := len(old), len(next)
	if n == 0 || m == 0 {
		return m, n, false
	}
	seen := make(map[string]bool, n)
	for _, line := range old {
		seen[line] = true
	}
	shared := false
	for _, line := range next {
		if seen[line] {
			shared = true
			break
		}
	}
	if !shared {
		return m, n, false
	}
	limit := n + m
	offset := limit + 1
	frontier := make([]int, 2*limit+3)
	work := 0
	for distance := 0; distance <= limit; distance++ {
		for diagonal := -distance; diagonal <= distance; diagonal += 2 {
			work++
			if work > 4_000_000 {
				return m, n, true
			}
			x := 0
			if diagonal == -distance || diagonal != distance && frontier[offset+diagonal-1] < frontier[offset+diagonal+1] {
				x = frontier[offset+diagonal+1]
			} else {
				x = frontier[offset+diagonal-1] + 1
			}
			y := x - diagonal
			for x < n && y < m && old[x] == next[y] {
				x++
				y++
				work++
			}
			frontier[offset+diagonal] = x
			if x >= n && y >= m {
				common := (n + m - distance) / 2
				return m - common, n - common, false
			}
		}
	}
	return m, n, true
}
