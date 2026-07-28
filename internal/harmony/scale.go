package harmony

import (
	"fmt"
	"sort"
	"strings"
)

type Scale struct {
	Name      string
	Intervals []int
}

var scaleTable = map[string]Scale{
	"minor_pentatonic": {Name: "minor_pentatonic", Intervals: []int{0, 3, 5, 7, 10}},
	"major_pentatonic": {Name: "major_pentatonic", Intervals: []int{0, 2, 4, 7, 9}},
	"dorian":           {Name: "dorian", Intervals: []int{0, 2, 3, 5, 7, 9, 10}},
	"aeolian":          {Name: "aeolian", Intervals: []int{0, 2, 3, 5, 7, 8, 10}},
	"major":            {Name: "major", Intervals: []int{0, 2, 4, 5, 7, 9, 11}},
	"lydian":           {Name: "lydian", Intervals: []int{0, 2, 4, 6, 7, 9, 11}},
	"phrygian":         {Name: "phrygian", Intervals: []int{0, 1, 3, 5, 7, 8, 10}},
}

func normalizeName(s string) string {
	return strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(s))
}

func LookupScale(name string) (Scale, error) {
	s, ok := scaleTable[normalizeName(name)]
	if !ok {
		return Scale{}, fmt.Errorf("unknown scale %q; valid names: %s", name, strings.Join(ScaleNames(), ", "))
	}
	intervals := make([]int, len(s.Intervals))
	copy(intervals, s.Intervals)
	return Scale{Name: s.Name, Intervals: intervals}, nil
}

func ScaleNames() []string {
	names := make([]string, 0, len(scaleTable))
	for k := range scaleTable {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func (s Scale) Degree(root Pitch, n int) Pitch {
	l := len(s.Intervals)
	octaves := n / l
	idx := n % l
	if idx < 0 {
		idx += l
		octaves--
	}
	semitones := s.Intervals[idx] + octaves*12
	return root.Transpose(semitones)
}
