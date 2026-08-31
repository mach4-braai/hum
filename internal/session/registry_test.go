package session

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mach4-braai/hum/internal/protocol"
)

func prd14Event(evType protocol.EventType, id string) protocol.Event {
	return protocol.Event{Event: evType, ID: id}
}

func startEvent(id, workspace, title string, priority int, meta map[string]string) protocol.Event {
	return protocol.Event{
		Event:     protocol.SessionStarted,
		ID:        id,
		Workspace: workspace,
		Title:     title,
		Priority:  priority,
		Metadata:  meta,
	}
}

func TestRegistryPRD14Sequence(t *testing.T) {
	r := New()

	c1, err := r.Apply(protocol.Event{
		Event:     protocol.SessionStarted,
		ID:        "123",
		Workspace: "tofu",
		Title:     "Validate PR #142",
	})
	if err != nil {
		t.Fatalf("Apply started: %v", err)
	}
	if c1.Kind != ChangeAdded {
		t.Errorf("first change kind = %q, want %q", c1.Kind, ChangeAdded)
	}

	c2, err := r.Apply(protocol.Event{Event: protocol.SessionCompleted, ID: "123"})
	if err != nil {
		t.Fatalf("Apply completed: %v", err)
	}
	if c2.Kind != ChangeEnded {
		t.Errorf("second change kind = %q, want %q", c2.Kind, ChangeEnded)
	}

	_, err = r.Apply(protocol.Event{Event: protocol.SessionCompleted, ID: "123"})
	if !errors.Is(err, ErrAlreadyTerminal) {
		t.Errorf("second completed: error = %v, want ErrAlreadyTerminal", err)
	}
}

func TestRegistryDuplicateActiveID(t *testing.T) {
	r := New()
	ev := startEvent("abc", "ws", "title", 0, nil)
	if _, err := r.Apply(ev); err != nil {
		t.Fatalf("first start: %v", err)
	}
	_, err := r.Apply(ev)
	if !errors.Is(err, ErrDuplicateID) {
		t.Errorf("duplicate start: error = %v, want ErrDuplicateID", err)
	}
}

func TestRegistryRestartTerminal(t *testing.T) {
	r := New()
	if _, err := r.Apply(startEvent("x", "ws", "t", 0, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Apply(prd14Event(protocol.SessionCompleted, "x")); err != nil {
		t.Fatal(err)
	}
	c, err := r.Apply(startEvent("x", "ws2", "t2", 1, nil))
	if err != nil {
		t.Fatalf("restart after terminal: %v", err)
	}
	if c.Kind != ChangeAdded {
		t.Errorf("restart change kind = %q, want %q", c.Kind, ChangeAdded)
	}
	snaps := r.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snaps))
	}
	if snaps[0].State != StateActive {
		t.Errorf("restarted session state = %q, want active", snaps[0].State)
	}
	if snaps[0].Updates != 0 {
		t.Errorf("restarted session Updates = %d, want 0", snaps[0].Updates)
	}
}

func TestRegistryUnknownSession(t *testing.T) {
	r := New()
	_, err := r.Apply(prd14Event(protocol.SessionUpdated, "nope"))
	if !errors.Is(err, ErrUnknownSession) {
		t.Errorf("updated unknown: error = %v, want ErrUnknownSession", err)
	}
	_, err = r.Apply(prd14Event(protocol.SessionCompleted, "nope"))
	if !errors.Is(err, ErrUnknownSession) {
		t.Errorf("completed unknown: error = %v, want ErrUnknownSession", err)
	}
}

func TestRegistryApplyValidation(t *testing.T) {
	r := New()
	_, err := r.Apply(protocol.Event{Event: "bad.event", ID: "1"})
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
	_, err = r.Apply(protocol.Event{Event: protocol.SessionStarted, ID: ""})
	if err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestRegistryUpdatedMerge(t *testing.T) {
	r := New()
	_, err := r.Apply(startEvent("m", "ws1", "title1", 0, map[string]string{"a": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Apply(protocol.Event{
		Event:    protocol.SessionUpdated,
		ID:       "m",
		Title:    "title2",
		Metadata: map[string]string{"b": "2"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if c.Session.Title != "title2" {
		t.Errorf("title after update = %q, want title2", c.Session.Title)
	}
	if c.Session.Metadata["a"] != "1" {
		t.Errorf("metadata a = %q, want 1", c.Session.Metadata["a"])
	}
	if c.Session.Metadata["b"] != "2" {
		t.Errorf("metadata b = %q, want 2", c.Session.Metadata["b"])
	}
	if c.Session.Updates != 1 {
		t.Errorf("Updates = %d, want 1", c.Session.Updates)
	}
}

func TestRegistryUpdateAddsMetadataToBareSession(t *testing.T) {
	r := New()
	if _, err := r.Apply(startEvent("bare", "ws", "t", 0, nil)); err != nil {
		t.Fatal(err)
	}

	c, err := r.Apply(protocol.Event{
		Event:    protocol.SessionUpdated,
		ID:       "bare",
		Metadata: map[string]string{"agents": "3"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if c.Session.Metadata["agents"] != "3" {
		t.Errorf("metadata agents = %q, want 3", c.Session.Metadata["agents"])
	}

	c, err = r.Apply(protocol.Event{Event: protocol.SessionUpdated, ID: "bare"})
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if len(c.Session.Metadata) != 1 {
		t.Errorf("metadata after an update carrying none = %v, want the earlier key retained", c.Session.Metadata)
	}
}

func TestRegistryUpdatedPreservesWorkspace(t *testing.T) {
	r := New()
	_, err := r.Apply(startEvent("w", "ws-original", "t", 0, nil))
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Apply(protocol.Event{
		Event: protocol.SessionUpdated,
		ID:    "w",
		Title: "new-title",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Session.Workspace != "ws-original" {
		t.Errorf("workspace changed to %q, want ws-original", c.Session.Workspace)
	}
}

func TestRegistryUpdatedRelabelsWorkspace(t *testing.T) {
	r := New()
	_, err := r.Apply(startEvent("rw", "ws-original", "t", 0, nil))
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.Apply(protocol.Event{
		Event:     protocol.SessionUpdated,
		ID:        "rw",
		Workspace: "ws-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Session.Workspace != "ws-new" {
		t.Errorf("workspace = %q, want ws-new", c.Session.Workspace)
	}
	c, err = r.Apply(protocol.Event{
		Event: protocol.SessionUpdated,
		ID:    "rw",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Session.Workspace != "ws-new" {
		t.Errorf("workspace after empty update = %q, want ws-new", c.Session.Workspace)
	}
}

func TestSnapshotDeepCopy(t *testing.T) {
	r := New()
	_, err := r.Apply(startEvent("dc", "ws", "t", 0, map[string]string{"k": "v"}))
	if err != nil {
		t.Fatal(err)
	}

	snap := r.Snapshot()
	snap[0].Title = "mutated"
	snap[0].Metadata["k"] = "mutated"

	snap2 := r.Snapshot()
	if snap2[0].Title == "mutated" {
		t.Error("mutating snapshot title affected registry")
	}
	if snap2[0].Metadata["k"] == "mutated" {
		t.Error("mutating snapshot metadata affected registry")
	}
}

func TestSnapshotOrder(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	counter := 0
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time {
		t := base.Add(time.Duration(counter) * time.Second)
		counter++
		return t
	}

	r := New()
	for _, id := range []string{"c", "a", "b"} {
		if _, err := r.Apply(startEvent(id, "", "", 0, nil)); err != nil {
			t.Fatal(err)
		}
	}

	snap := r.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("snapshot len = %d, want 3", len(snap))
	}
	if snap[0].ID != "c" || snap[1].ID != "a" || snap[2].ID != "b" {
		t.Errorf("order = [%s %s %s], want [c a b]", snap[0].ID, snap[1].ID, snap[2].ID)
	}
}

func TestSnapshotSameStartedAtSortsByID(t *testing.T) {
	fixed := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return fixed }

	r := New()
	for _, id := range []string{"z", "a", "m"} {
		if _, err := r.Apply(startEvent(id, "", "", 0, nil)); err != nil {
			t.Fatal(err)
		}
	}

	snap := r.Snapshot()
	if snap[0].ID != "a" || snap[1].ID != "m" || snap[2].ID != "z" {
		t.Errorf("order = [%s %s %s], want [a m z]", snap[0].ID, snap[1].ID, snap[2].ID)
	}
}

func TestReap(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	current := base
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return current }

	r := New()

	if _, err := r.Apply(startEvent("active1", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Apply(startEvent("done1", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}
	current = base.Add(1 * time.Second)
	if _, err := r.Apply(prd14Event(protocol.SessionCompleted, "done1")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Apply(startEvent("done2", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}
	current = base.Add(2 * time.Second)
	if _, err := r.Apply(prd14Event(protocol.SessionFailed, "done2")); err != nil {
		t.Fatal(err)
	}

	current = base.Add(10 * time.Second)
	dropped := r.Reap(5 * time.Second)
	if dropped != 2 {
		t.Errorf("Reap dropped %d, want 2", dropped)
	}

	snap := r.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot after reap len = %d, want 1", len(snap))
	}
	if snap[0].ID != "active1" {
		t.Errorf("remaining session = %q, want active1", snap[0].ID)
	}
}

func TestReapDoesNotTouchActiveSessions(t *testing.T) {
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC) }

	r := New()
	if _, err := r.Apply(startEvent("alive", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}

	now = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	dropped := r.Reap(1 * time.Second)
	if dropped != 0 {
		t.Errorf("Reap dropped %d active sessions, want 0 — active sessions must travel the cancellation path, not be deleted directly", dropped)
	}
}

func startEventWithOwner(id, host string, pid int) protocol.Event {
	return protocol.Event{Event: protocol.SessionStarted, ID: id, OwnerPID: pid, OwnerHost: host}
}

func TestActiveCancelDeadOwner(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	pidAlive = func(int) bool { return false }

	r := New()
	if _, err := r.Apply(startEventWithOwner("s1", "myhost", 12345)); err != nil {
		t.Fatal(err)
	}

	candidates := r.ActiveToCancel(0, "myhost")
	if len(candidates) != 1 {
		t.Fatalf("ActiveToCancel returned %d candidates, want 1", len(candidates))
	}
	if candidates[0].ID != "s1" {
		t.Errorf("candidate id = %q, want s1", candidates[0].ID)
	}
	if candidates[0].Reason == "" {
		t.Error("candidate reason is empty")
	}
}

func TestActiveCancelAliveOwner(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	pidAlive = func(int) bool { return true }

	r := New()
	if _, err := r.Apply(startEventWithOwner("s1", "myhost", 12345)); err != nil {
		t.Fatal(err)
	}

	candidates := r.ActiveToCancel(0, "myhost")
	if len(candidates) != 0 {
		t.Errorf("ActiveToCancel returned %d candidates for alive owner, want 0", len(candidates))
	}
}

func TestActiveCancelHostMismatch(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	pidAlive = func(int) bool { return false }

	r := New()
	if _, err := r.Apply(startEventWithOwner("s1", "otherhost", 12345)); err != nil {
		t.Fatal(err)
	}

	candidates := r.ActiveToCancel(0, "myhost")
	if len(candidates) != 0 {
		t.Errorf("ActiveToCancel returned %d candidates for host mismatch, want 0", len(candidates))
	}
}

func TestActiveCancelNoOwnerNoPid(t *testing.T) {
	r := New()
	if _, err := r.Apply(startEvent("s1", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}

	candidates := r.ActiveToCancel(0, "myhost")
	if len(candidates) != 0 {
		t.Errorf("ActiveToCancel returned %d candidates for ownerless session with disabled lease, want 0", len(candidates))
	}
}

func TestActiveCancelLeaseExpired(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	current := base
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return current }

	r := New()
	if _, err := r.Apply(startEvent("s1", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}

	const lease = time.Hour
	current = base.Add(2 * time.Hour)
	candidates := r.ActiveToCancel(lease, "myhost")
	if len(candidates) != 1 {
		t.Fatalf("ActiveToCancel returned %d candidates, want 1 for expired lease", len(candidates))
	}
	if candidates[0].ID != "s1" {
		t.Errorf("candidate id = %q, want s1", candidates[0].ID)
	}
}

func TestActiveCancelLeaseNotExpired(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	current := base
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return current }

	r := New()
	if _, err := r.Apply(startEvent("s1", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}

	const lease = time.Hour
	current = base.Add(30 * time.Minute)
	candidates := r.ActiveToCancel(lease, "myhost")
	if len(candidates) != 0 {
		t.Errorf("ActiveToCancel returned %d candidates, want 0 for unexpired lease", len(candidates))
	}
}

func TestActiveCancelLeaseRefreshedByUpdate(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	current := base
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return current }

	r := New()
	if _, err := r.Apply(startEvent("s1", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}

	const lease = time.Hour
	current = base.Add(90 * time.Minute)
	if _, err := r.Apply(protocol.Event{Event: protocol.SessionUpdated, ID: "s1"}); err != nil {
		t.Fatal(err)
	}

	current = base.Add(2 * time.Hour)
	candidates := r.ActiveToCancel(lease, "myhost")
	if len(candidates) != 0 {
		t.Errorf("ActiveToCancel returned %d candidates after lease refresh, want 0", len(candidates))
	}
}

func TestActiveCancelTerminalSessionSkipped(t *testing.T) {
	old := pidAlive
	t.Cleanup(func() { pidAlive = old })
	pidAlive = func(int) bool { return false }

	r := New()
	if _, err := r.Apply(startEventWithOwner("s1", "myhost", 12345)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Apply(protocol.Event{Event: protocol.SessionCompleted, ID: "s1"}); err != nil {
		t.Fatal(err)
	}

	candidates := r.ActiveToCancel(0, "myhost")
	if len(candidates) != 0 {
		t.Errorf("ActiveToCancel returned %d candidates for terminal session, want 0", len(candidates))
	}
}

func TestRegistryConcurrent(t *testing.T) {
	r := New()
	const goroutines = 8
	const ops = 100

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := "sess"
			for i := range ops {
				switch i % 4 {
				case 0:
					r.Apply(startEvent(id, "ws", "t", g, nil))
				case 1:
					r.Apply(protocol.Event{Event: protocol.SessionUpdated, ID: id, Title: "updated"})
				case 2:
					r.Snapshot()
				case 3:
					r.Apply(prd14Event(protocol.SessionCompleted, id))
					r.Reap(0)
				}
			}
		}()
	}
	wg.Wait()
}

func TestChangeZeroOnError(t *testing.T) {
	r := New()
	ch, err := r.Apply(prd14Event(protocol.SessionCompleted, "unknown"))
	if !errors.Is(err, ErrUnknownSession) {
		t.Errorf("error = %v, want ErrUnknownSession", err)
	}
	if ch.Kind != "" || ch.Session.ID != "" || ch.Prev != "" {
		t.Errorf("change = %+v, want zero", ch)
	}
}

func TestStartPopulatesFields(t *testing.T) {
	r := New()
	meta := map[string]string{"env": "prod"}
	ch, err := r.Apply(startEvent("p", "my-workspace", "My Task", 3, meta))
	if err != nil {
		t.Fatal(err)
	}
	s := ch.Session
	if s.Workspace != "my-workspace" {
		t.Errorf("Workspace = %q", s.Workspace)
	}
	if s.Title != "My Task" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.Priority != 3 {
		t.Errorf("Priority = %d", s.Priority)
	}
	if s.Metadata["env"] != "prod" {
		t.Errorf("Metadata[env] = %q", s.Metadata["env"])
	}
	meta["env"] = "mutated"
	if s.Metadata["env"] == "mutated" {
		t.Error("change session metadata aliased the input map")
	}
}

func TestEndedAtSetOnTerminal(t *testing.T) {
	fixed := time.Date(2024, 6, 1, 10, 0, 0, 0, time.UTC)
	endTime := fixed.Add(30 * time.Second)
	current := fixed
	old := now
	t.Cleanup(func() { now = old })
	now = func() time.Time { return current }

	r := New()
	if _, err := r.Apply(startEvent("e", "", "", 0, nil)); err != nil {
		t.Fatal(err)
	}
	current = endTime
	ch, err := r.Apply(prd14Event(protocol.SessionCompleted, "e"))
	if err != nil {
		t.Fatal(err)
	}
	if ch.Session.EndedAt != endTime {
		t.Errorf("EndedAt = %v, want %v", ch.Session.EndedAt, endTime)
	}
}
