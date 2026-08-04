//go:build e2e

package e2e

import "testing"

func TestTwoConcurrentProjectsShareTheFirstContext(t *testing.T) {
	d := start(t)

	alpha := writeProjectConfig(t, "project:\n  name: alpha\nmusic:\n  root: D\n  octave: 3\n  scale: dorian\n")
	beta := writeProjectConfig(t, "project:\n  name: beta\nmusic:\n  root: A\n  octave: 3\n  scale: minor_pentatonic\n")

	d.mustHum(t, "start", "--id", "a1", "--title", "alpha work", "--root", alpha)

	st := d.status(t)
	if st.Scale != "dorian" || st.Root != "D3" {
		t.Fatalf("context = %s %s, want D3 dorian from alpha", st.Root, st.Scale)
	}
	if st.ContextOwner != alpha {
		t.Fatalf("context owner = %q, want %q", st.ContextOwner, alpha)
	}

	d.mustHum(t, "start", "--id", "b1", "--title", "beta work", "--root", beta)

	st = d.status(t)
	if st.Scale != "dorian" || st.Root != "D3" {
		t.Errorf("context = %s %s after beta joined, want alpha's D3 dorian: a joining session inherits the sounding context", st.Root, st.Scale)
	}
	if st.ContextOwner != alpha {
		t.Errorf("context owner = %q after beta joined, want %q", st.ContextOwner, alpha)
	}
	if st.SoundingVoices != 2 {
		t.Errorf("sounding voices = %d, want 2", st.SoundingVoices)
	}

	dorianSecond := pitches(st)["b1"]
	if dorianSecond != "F3" {
		t.Errorf("beta's session sounds %q, want F3: the second dorian voice is the minor third, voiced beside the root", dorianSecond)
	}
}

func TestAnEmptySoundscapeAdoptsTheNextProject(t *testing.T) {
	d := start(t)

	alpha := writeProjectConfig(t, "project:\n  name: alpha\nmusic:\n  root: D\n  octave: 3\n  scale: dorian\n")
	beta := writeProjectConfig(t, "project:\n  name: beta\nmusic:\n  root: A\n  octave: 3\n  scale: minor_pentatonic\n")

	d.mustHum(t, "start", "--id", "a1", "--root", alpha)
	d.mustHum(t, "start", "--id", "b1", "--root", beta)
	d.mustHum(t, "complete", "--id", "a1")
	d.mustHum(t, "complete", "--id", "b1")

	if got := d.status(t).SoundingVoices; got != 0 {
		t.Fatalf("sounding voices = %d, want 0 before the next adoption", got)
	}

	d.mustHum(t, "start", "--id", "b2", "--root", beta)

	st := d.status(t)
	if st.Root != "A3" || st.Scale != "minor_pentatonic" {
		t.Errorf("context = %s %s, want A3 minor_pentatonic: an empty soundscape adopts the joining project", st.Root, st.Scale)
	}
	if st.ContextOwner != beta {
		t.Errorf("context owner = %q, want %q", st.ContextOwner, beta)
	}
}

func TestASessionWithNoProjectRootUsesGlobalConfig(t *testing.T) {
	d := start(t)

	if resp := d.sendRaw(t, `{"event":"session.started","id":"bare","title":"no root"}`); !resp.OK {
		t.Fatalf("a session with no root was rejected: %s", resp.Error)
	}

	st := d.status(t)
	if st.Root != "C4" || st.Scale != "major" {
		t.Errorf("context = %s %s, want the default C4 major", st.Root, st.Scale)
	}
	if st.ContextOwner != "" {
		t.Errorf("context owner = %q, want empty: no project claimed the context", st.ContextOwner)
	}
}

func TestARootThatDoesNotExistIsRejected(t *testing.T) {
	d := start(t)

	resp := d.sendRaw(t, `{"event":"session.started","id":"typo","root":"/nope/not/here"}`)
	if resp.OK {
		t.Fatal("a nonexistent project root was accepted; a typo must not masquerade as no project config")
	}
	if resp.Error == "" {
		t.Error("the rejection carries no error text")
	}

	if got := d.status(t).SoundingVoices; got != 0 {
		t.Errorf("sounding voices = %d after a rejected start, want 0", got)
	}
}

func TestASymlinkedRootIsTheSameProject(t *testing.T) {
	d := start(t)

	alpha := writeProjectConfig(t, "project:\n  name: alpha\nmusic:\n  root: D\n  scale: dorian\n")
	link := symlink(t, alpha)

	d.mustHum(t, "start", "--id", "direct", "--root", alpha)
	d.mustHum(t, "start", "--id", "linked", "--root", link)

	st := d.status(t)
	if st.ContextOwner != alpha {
		t.Errorf("context owner = %q, want the canonical %q: a symlinked path must not become a second project", st.ContextOwner, alpha)
	}
	if st.SoundingVoices != 2 {
		t.Errorf("sounding voices = %d, want 2", st.SoundingVoices)
	}
}
