package harmony

import (
	"math"
	"strconv"
	"time"
)

var now = time.Now

const (
	exprHalfLife     = 5 * time.Second
	exprIntensityCap = 10.0
	exprAgentsCap    = 20.0
)

type Expression struct {
	Intensity float64
	Tremolo   float64
	Width     float64
}

type exprTracker struct {
	score    float64
	lastTime time.Time
	agents   int
}

func (t *exprTracker) record() {
	n := now()
	if !t.lastTime.IsZero() {
		dt := n.Sub(t.lastTime).Seconds()
		lambda := math.Log(2) / exprHalfLife.Seconds()
		t.score = t.score*math.Exp(-lambda*dt) + 1.0
	} else {
		t.score = 1.0
	}
	t.lastTime = n
}

func (t *exprTracker) current() Expression {
	if t == nil {
		return Expression{}
	}
	score := t.score
	if !t.lastTime.IsZero() {
		dt := now().Sub(t.lastTime).Seconds()
		lambda := math.Log(2) / exprHalfLife.Seconds()
		score = score * math.Exp(-lambda*dt)
	}
	intensity := math.Min(score/exprIntensityCap, 1.0)
	return Expression{
		Intensity: intensity,
		Tremolo:   intensity,
		Width:     math.Min(float64(t.agents-1)/exprAgentsCap, 1.0),
	}
}

func agentsFromMetadata(m map[string]string) int {
	s, ok := m["agents"]
	if !ok {
		return 1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return n
}
