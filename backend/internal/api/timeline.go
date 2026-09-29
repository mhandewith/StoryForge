package api

import "sort"

// timelineLine is the single source of truth for preview and export positions.
type timelineLine struct {
	ID, Element          string
	Duration, Start, End int
}
type timelineElement struct {
	Key      string
	Lines    []timelineLine
	Position int
}

// calculateTimeline accepts persisted relationships, rather than timestamps.
// It is intentionally pure so every caller obtains the same arrangement.
func calculateTimeline(elements []timelineElement, transitions map[string]int) []timelineLine {
	sort.SliceStable(elements, func(i, j int) bool { return elements[i].Position < elements[j].Position })
	cursor := 0
	result := []timelineLine{}
	for i, e := range elements {
		if i > 0 {
			cursor += transitions[elements[i-1].Key+"\x00"+e.Key]
		}
		if cursor < 0 {
			cursor = 0
		}
		duration := 0
		for _, l := range e.Lines {
			l.Start = cursor + l.Start
			l.End = l.Start + l.Duration
			if l.End-cursor > duration {
				duration = l.End - cursor
			}
			result = append(result, l)
		}
		cursor += duration
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Start < result[j].Start })
	return result
}
