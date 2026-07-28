package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/protocol"
)

const statusBasePayload = `,"theme":"default","root":"C4","scale":"major","renderer":"nop","sample_rate":44100,"version":"1.0","volume":0.8,"muted":false,"sounding_voices":0}`

func statusOKResponse(sessionsJSON string) string {
	return `{"ok":true,"data":{"sessions":[` + sessionsJSON + `]` + statusBasePayload + `}` + "\n"
}

func TestStatusTableAligns(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	resp := statusOKResponse(
		`{"id":"abc-def","workspace":"ws1","title":"Short","state":"active","pitch":"C4","seconds":30,"updates":0},` +
			`{"id":"x","workspace":"long-workspace","title":"Much longer title here","state":"completed","seconds":3661,"updates":0}`,
	)
	serveOne(t, resp)
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 output lines, got %d:\n%s", len(lines), stdout.String())
	}
	header, row1, row2 := lines[0], lines[1], lines[2]

	order := []string{"ID", "WORKSPACE", "TITLE", "STATE", "NOTE", "AGE"}
	prev := -1
	for _, col := range order {
		idx := strings.Index(header, col)
		if idx < 0 {
			t.Errorf("header missing column %q; header=%q", col, header)
			continue
		}
		if idx <= prev {
			t.Errorf("column %q out of order in header; header=%q", col, header)
		}
		prev = idx
	}

	wsPos := strings.Index(header, "WORKSPACE")
	for i, row := range []string{row1, row2} {
		ws := []string{"ws1", "long-workspace"}[i]
		if len(row) <= wsPos {
			t.Errorf("row %d too short for workspace column (len=%d, wsPos=%d)", i, len(row), wsPos)
			continue
		}
		if !strings.HasPrefix(row[wsPos:], ws) {
			t.Errorf("row %d workspace not aligned: want %q at col %d, got %q", i, ws, wsPos, row[wsPos:])
		}
	}
}

func TestStatusNoteColumn(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	resp := statusOKResponse(
		`{"id":"a","workspace":"ws","title":"T","state":"active","pitch":"E4","seconds":5,"updates":0},` +
			`{"id":"b","workspace":"ws","title":"T2","state":"completed","seconds":10,"updates":0}`,
	)
	serveOne(t, resp)
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d stderr=%q", code, stderr.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	notePos := strings.Index(lines[0], "NOTE")
	if notePos < 0 {
		t.Fatal("NOTE column missing from header")
	}
	if !strings.HasPrefix(lines[1][notePos:], "E4") {
		t.Errorf("active session NOTE: want E4, got %q", lines[1][notePos:])
	}
	if !strings.HasPrefix(lines[2][notePos:], "-") {
		t.Errorf("completed session NOTE: want -, got %q", lines[2][notePos:])
	}
}

func TestStatusJSON(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	resp := statusOKResponse(
		`{"id":"a","workspace":"ws","title":"T","state":"active","pitch":"G#3","seconds":5,"updates":0}`,
	)
	serveOne(t, resp)
	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--json"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d stderr=%q", code, stderr.String())
	}
	var payload protocol.StatusPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &payload); err != nil {
		t.Fatalf("stdout is not valid StatusPayload JSON: %v\nstdout=%q", err, stdout.String())
	}
	if len(payload.Sessions) != 1 {
		t.Fatalf("want 1 session, got %d", len(payload.Sessions))
	}
	if payload.Sessions[0].Pitch != "G#3" {
		t.Errorf("want pitch G#3, got %q", payload.Sessions[0].Pitch)
	}
	if strings.Contains(stdout.String(), "WORKSPACE") {
		t.Error("table header should not appear in --json output")
	}
}

func TestStatusEmptySessions(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	resp := `{"ok":true,"data":{"sessions":[]` + statusBasePayload + `}` + "\n"
	serveOne(t, resp)
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	if got := strings.TrimRight(stdout.String(), "\n"); got != "no active sessions" {
		t.Errorf("want %q, got %q", "no active sessions", got)
	}
}

func TestStatusPipedUntruncated(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	longTitle := strings.Repeat("X", 200)
	resp := statusOKResponse(
		`{"id":"a","workspace":"ws","title":"` + longTitle + `","state":"active","pitch":"C4","seconds":1,"updates":0}`,
	)
	serveOne(t, resp)
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), longTitle) {
		t.Errorf("long title was truncated in piped output; title len=%d, stdout len=%d", len(longTitle), len(stdout.String()))
	}
}

func TestStatusTruncate(t *testing.T) {
	if got := statusTruncate("hi", 10); got != "hi" {
		t.Errorf("short title: want %q, got %q", "hi", got)
	}

	got := statusTruncate("Hello World Foo", 8)
	runes := []rune(got)
	if len(runes) != 8 {
		t.Errorf("truncated to %d runes, want 8", len(runes))
	}
	if runes[len(runes)-1] != '…' {
		t.Errorf("last rune: want '…', got %q", string(runes[len(runes)-1:]))
	}

	got4 := statusTruncate("ABCDEFGH", 4)
	runes4 := []rune(got4)
	if len(runes4) != 4 || runes4[3] != '…' {
		t.Errorf("truncate to 4: want 4 runes ending with '…', got %q", got4)
	}

	multi := statusTruncate("hé world test", 3)
	runesM := []rune(multi)
	if len(runesM) != 3 {
		t.Errorf("multi-byte: got %d runes, want 3", len(runesM))
	}
	if runesM[2] != '…' {
		t.Errorf("multi-byte: want trailing '…', got %q", string(runesM[2:]))
	}
	if runesM[1] != 'é' {
		t.Errorf("multi-byte: rune 1 should be 'é', got %q (byte split would corrupt it)", string(runesM[1:2]))
	}

	if got := statusTruncate("anything", 0); got != "anything" {
		t.Errorf("zero maxRunes: want unchanged, got %q", got)
	}
	if got := statusTruncate("anything", -1); got != "anything" {
		t.Errorf("negative maxRunes: want unchanged, got %q", got)
	}
}

func TestStatusAge(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0, "0s"},
		{1, "1s"},
		{59, "59s"},
		{60, "1m00s"},
		{61, "1m01s"},
		{3599, "59m59s"},
		{3600, "1h00m"},
		{3661, "1h01m"},
		{7322, "2h02m"},
	}
	for _, tt := range tests {
		if got := statusAge(tt.seconds); got != tt.want {
			t.Errorf("statusAge(%g) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestStatusAbsentDaemon(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("exit %d, want %d (exitUnreachable)", code, exitUnreachable)
	}
}

func TestStatusDaemonError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	serveOne(t, `{"ok":false,"error":"internal daemon error"}`+"\n")
	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitDaemonError {
		t.Errorf("exit %d, want %d (exitDaemonError)", code, exitDaemonError)
	}
	if !strings.Contains(stderr.String(), "internal daemon error") {
		t.Errorf("stderr missing error message, got %q", stderr.String())
	}
}

func TestStatusStrayArgument(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "extra"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("exit %d, want %d (exitUsage)", code, exitUsage)
	}
}

func TestStatusTruncatesOnTerminal(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	longTitle := strings.Repeat("Z", 100)
	resp := statusOKResponse(
		`{"id":"long-session-id","workspace":"long-workspace-name","title":"` + longTitle + `","state":"active","pitch":"C4","seconds":61,"updates":0},` +
			`{"id":"x","workspace":"w","title":"short","state":"completed","seconds":1,"updates":0}`,
	)
	serveOne(t, resp)

	t.Cleanup(func() { statusWidthFn = statusWidth })
	statusWidthFn = func(_ io.Writer) int { return 120 }

	var stdout, stderr bytes.Buffer
	code := run([]string{"status"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), longTitle) {
		t.Errorf("long title was not truncated at terminal width 120")
	}
	if !strings.Contains(stdout.String(), "…") {
		t.Errorf("expected ellipsis in truncated output, got %q", stdout.String())
	}
}

func TestStatusWidthOnPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	if got := statusWidth(w); got != 0 {
		t.Errorf("statusWidth on pipe: want 0, got %d", got)
	}
}

func TestStatusBadFlagIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := run([]string{"status", "--bad-flag"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("exit %d, want %d (exitUsage)", code, exitUsage)
	}
}
