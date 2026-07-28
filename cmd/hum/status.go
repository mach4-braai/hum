package main

import (
	"fmt"
	"io"
	"os"
	"syscall"
	"text/tabwriter"
	"unicode/utf8"
	"unsafe"

	"github.com/mach4-braai/hum/internal/protocol"
)

func init() {
	register("status", runStatus)
}

type statusRow struct {
	id        string
	workspace string
	title     string
	state     string
	note      string
	age       string
}

func statusAge(seconds float64) string {
	s := int(seconds)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	if m < 60 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh%02dm", h, m)
}

func statusTruncate(title string, maxRunes int) string {
	if maxRunes <= 0 {
		return title
	}
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return string(runes[:maxRunes-1]) + "…"
}

func statusWidth(w io.Writer) int {
	file, ok := w.(*os.File)
	if !ok {
		return 0
	}
	var size struct{ rows, cols, xpixel, ypixel uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 || size.cols == 0 {
		return 0
	}
	return int(size.cols)
}

var statusWidthFn = statusWidth

func statusRows(sessions []protocol.SessionPayload) []statusRow {
	rows := make([]statusRow, len(sessions))
	for i, s := range sessions {
		note := s.Pitch
		if note == "" {
			note = "-"
		}
		rows[i] = statusRow{
			id:        s.ID,
			workspace: s.Workspace,
			title:     s.Title,
			state:     s.State,
			note:      note,
			age:       statusAge(s.Seconds),
		}
	}
	return rows
}

func statusFitTitles(rows []statusRow, width int) {
	if width <= 0 {
		return
	}

	const pad = 2
	widest := func(header string, of func(statusRow) string) int {
		max := utf8.RuneCountInString(header)
		for _, r := range rows {
			if n := utf8.RuneCountInString(of(r)); n > max {
				max = n
			}
		}
		return max
	}

	fixed := widest("ID", func(r statusRow) string { return r.id }) + pad
	fixed += widest("WORKSPACE", func(r statusRow) string { return r.workspace }) + pad
	fixed += widest("STATE", func(r statusRow) string { return r.state }) + pad
	fixed += widest("NOTE", func(r statusRow) string { return r.note }) + pad
	fixed += widest("AGE", func(r statusRow) string { return r.age }) + pad

	for i := range rows {
		rows[i].title = statusTruncate(rows[i].title, width-fixed)
	}
}

func runStatus(e *env, words []string) int {
	rest, ok := operands(e, "status", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "status", rest[0])
	}

	payload, raw, code := fetchStatus(e)
	if code != exitOK {
		return code
	}

	if e.opts.asJSON {
		printJSON(e, raw)
		return exitOK
	}

	if len(payload.Sessions) == 0 {
		fmt.Fprintln(e.stdout, "no active sessions")
		return exitOK
	}

	rows := statusRows(payload.Sessions)
	statusFitTitles(rows, statusWidthFn(e.stdout))

	tw := tabwriter.NewWriter(e.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tWORKSPACE\tTITLE\tSTATE\tNOTE\tAGE")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.id, r.workspace, r.title, r.state, r.note, r.age)
	}
	tw.Flush()
	return exitOK
}
