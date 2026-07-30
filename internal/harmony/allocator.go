package harmony

import (
	"sort"
	"sync"
)

const MaxVoices = 12

type Voice struct {
	SessionID string
	Degree    int
	Pitch     Pitch
}

type Allocator struct {
	mu     sync.Mutex
	root   Pitch
	scale  Scale
	voices map[string]Voice
	free   []int
	capped map[string]bool
}

func NewAllocator(root Pitch, scale Scale) *Allocator {
	free := make([]int, MaxVoices)
	for i := range free {
		free[i] = i
	}
	return &Allocator{
		root:   root,
		scale:  scale,
		voices: make(map[string]Voice),
		free:   free,
		capped: make(map[string]bool),
	}
}

func (a *Allocator) Acquire(sessionID string) Voice {
	a.mu.Lock()
	defer a.mu.Unlock()
	if v, ok := a.voices[sessionID]; ok {
		return v
	}
	var degree int
	var isCapped bool
	if len(a.free) > 0 {
		degree = a.free[0]
		a.free = a.free[1:]
	} else {
		degree = MaxVoices - 1
		isCapped = true
	}
	v := Voice{
		SessionID: sessionID,
		Degree:    degree,
		Pitch:     voicing(a.root, a.scale, degree),
	}
	a.voices[sessionID] = v
	if isCapped {
		a.capped[sessionID] = true
	}
	return v
}

func (a *Allocator) Release(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.voices[sessionID]
	if !ok {
		return
	}
	delete(a.voices, sessionID)
	if a.capped[sessionID] {
		delete(a.capped, sessionID)
		return
	}
	idx := sort.SearchInts(a.free, v.Degree)
	a.free = append(a.free, 0)
	copy(a.free[idx+1:], a.free[idx:])
	a.free[idx] = v.Degree
}

func (a *Allocator) Active() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.voices)
}

func (a *Allocator) Voices() []Voice {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Voice, 0, len(a.voices))
	for _, v := range a.voices {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Degree == out[j].Degree {
			return out[i].SessionID < out[j].SessionID
		}
		return out[i].Degree < out[j].Degree
	})
	return out
}

func (a *Allocator) VoiceFor(sessionID string) (Voice, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.voices[sessionID]
	return v, ok
}

func voicing(root Pitch, scale Scale, degree int) Pitch {
	if degree <= 0 {
		return root
	}
	steps := len(scale.Intervals)
	return scale.Degree(root, (degree-1)%steps+1).Transpose(12)
}
