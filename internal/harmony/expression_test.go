package harmony

import (
	"math"
	"testing"
	"time"
)

func stageNow(t *testing.T, base time.Time) *time.Time {
	t.Helper()
	ts := base
	now = func() time.Time { return ts }
	t.Cleanup(func() { now = time.Now })
	return &ts
}

func TestExpressionZeroBeforeUpdates(t *testing.T) {
	tr := &exprTracker{agents: 1}
	expr := tr.current()
	if expr.Intensity != 0 || expr.Tremolo != 0 || expr.Width != 0 {
		t.Errorf("fresh tracker: want zero expression, got %+v", expr)
	}
}

func TestExpressionRaisesOnUpdates(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := stageNow(t, base)

	tr := &exprTracker{agents: 1}
	for range 10 {
		tr.record()
	}

	expr := tr.current()
	if expr.Intensity <= 0 {
		t.Errorf("after 10 updates: want Intensity > 0, got %v", expr.Intensity)
	}

	prev := expr.Intensity
	*ts = base.Add(5 * time.Second)
	expr2 := tr.current()
	if expr2.Intensity >= prev {
		t.Errorf("after half-life: want Intensity < %v, got %v", prev, expr2.Intensity)
	}

	*ts = base.Add(100 * time.Second)
	expr3 := tr.current()
	if expr3.Intensity >= expr2.Intensity {
		t.Errorf("after long wait: want Intensity < %v, got %v", expr2.Intensity, expr3.Intensity)
	}
}

func TestExpressionDecaysTowardZero(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := stageNow(t, base)

	tr := &exprTracker{agents: 1}
	tr.record()

	*ts = base.Add(1000 * time.Second)
	expr := tr.current()
	if expr.Intensity >= 0.001 {
		t.Errorf("after 1000s: want near-zero Intensity, got %v", expr.Intensity)
	}
}

func TestExpressionRecordFirstCall(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stageNow(t, base)

	tr := &exprTracker{agents: 1}
	tr.record()

	if tr.score != 1.0 {
		t.Errorf("first record: want score 1.0, got %v", tr.score)
	}
	if tr.lastTime != base {
		t.Errorf("first record: want lastTime %v, got %v", base, tr.lastTime)
	}
}

func TestExpressionRecordSubsequentDecays(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := stageNow(t, base)

	tr := &exprTracker{agents: 1}
	tr.record()

	*ts = base.Add(5 * time.Second)
	tr.record()

	if math.Abs(tr.score-1.5) > 1e-12 {
		t.Errorf("after half-life + new event: want score 1.5, got %v", tr.score)
	}
}

func TestExpressionAgentsRaisesWidth(t *testing.T) {
	tr := &exprTracker{agents: 11}
	expr := tr.current()
	if expr.Width <= 0 {
		t.Errorf("agents=11: want Width > 0, got %v", expr.Width)
	}

	tr2 := &exprTracker{agents: 1}
	expr2 := tr2.current()
	if expr2.Width != 0 {
		t.Errorf("agents=1: want Width 0, got %v", expr2.Width)
	}

	if expr.Width <= expr2.Width {
		t.Errorf("agents=11 should have higher Width than agents=1")
	}
}

func TestExpressionAgentsCapAt1(t *testing.T) {
	if agentsFromMetadata(nil) != 1 {
		t.Error("nil metadata: want agents=1")
	}
	if agentsFromMetadata(map[string]string{}) != 1 {
		t.Error("empty metadata: want agents=1")
	}
	if agentsFromMetadata(map[string]string{"agents": "bad"}) != 1 {
		t.Error("non-integer agents: want agents=1")
	}
	if agentsFromMetadata(map[string]string{"agents": "0"}) != 1 {
		t.Error("agents=0: want agents=1")
	}
	if agentsFromMetadata(map[string]string{"agents": "-5"}) != 1 {
		t.Error("agents=-5: want agents=1")
	}
	if agentsFromMetadata(map[string]string{"agents": "7"}) != 7 {
		t.Error("agents=7: want 7")
	}
	if agentsFromMetadata(map[string]string{"agents": "2"}) != 2 {
		t.Error("agents=2: want 2")
	}
}

func TestExpressionIntensityCapped(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stageNow(t, base)

	tr := &exprTracker{agents: 1}
	for range 100 {
		tr.record()
	}

	expr := tr.current()
	if expr.Intensity > 1.0 {
		t.Errorf("intensity must not exceed 1.0, got %v", expr.Intensity)
	}
	if expr.Intensity != 1.0 {
		t.Errorf("100 rapid updates: want intensity=1.0, got %v", expr.Intensity)
	}
}

func TestAbsentTrackerYieldsNeutralExpression(t *testing.T) {
	var absent *exprTracker
	if got := absent.current(); got != (Expression{}) {
		t.Errorf("current() on an absent tracker = %+v, want the neutral zero value", got)
	}
}

func TestExpressionHalfLifeDecayExact(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := stageNow(t, base)
	tr := &exprTracker{agents: 1}
	tr.record()
	*ts = base.Add(exprHalfLife)
	expr := tr.current()
	if math.Abs(expr.Intensity-0.05) > 1e-12 {
		t.Errorf("after one half-life: want Intensity 0.05, got %v", expr.Intensity)
	}
}

func TestExpressionIntensityCapExact(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stageNow(t, base)
	tr := &exprTracker{agents: 1}
	tr.record()
	expr := tr.current()
	if math.Abs(expr.Intensity-0.1) > 1e-12 {
		t.Errorf("immediately after record: want Intensity 0.1, got %v", expr.Intensity)
	}
}

func TestExpressionWidthCapExact(t *testing.T) {
	tr20 := &exprTracker{agents: 20}
	if w := tr20.current().Width; math.Abs(w-19.0/20.0) > 1e-14 {
		t.Errorf("agents=20: want Width %v, got %v", 19.0/20.0, w)
	}
	tr22 := &exprTracker{agents: 22}
	if w := tr22.current().Width; w != 1.0 {
		t.Errorf("agents=22: want Width 1.0 (clamped), got %v", w)
	}
}
