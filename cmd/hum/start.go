package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

func init() {
	register("start", runStart)
}

type startMetaFlag map[string]string

func (m startMetaFlag) String() string { return "" }

func (m *startMetaFlag) Set(s string) error {
	idx := strings.IndexByte(s, '=')
	if idx < 0 {
		return fmt.Errorf("meta %q: expected key=value", s)
	}
	key := s[:idx]
	if key == "" {
		return fmt.Errorf("meta: empty key in %q", s)
	}
	if *m == nil {
		*m = make(startMetaFlag)
	}
	(*m)[key] = s[idx+1:]
	return nil
}

func startRandomID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func runStart(e *env, words []string) int {
	var id, workspace, title, rootFlag string
	var priority int
	var meta startMetaFlag

	rest, ok := operands(e, "start", words, func(f *flag.FlagSet) {
		f.StringVar(&id, "id", id, "session id")
		f.StringVar(&workspace, "workspace", workspace, "workspace label")
		f.StringVar(&title, "title", title, "session title")
		f.StringVar(&rootFlag, "root", rootFlag, "project root directory")
		f.IntVar(&priority, "priority", priority, "session priority")
		f.Var(&meta, "meta", "k=v metadata")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "start", rest[0])
	}

	if id == "" {
		id = os.Getenv("HUM_SESSION_ID")
	}
	if id == "" {
		id = startRandomID()
	}

	root, err := paths.ProjectRoot(rootFlag)
	if err != nil {
		if rootFlag != "" {
			return e.usagef("hum start: --root %q: %v", rootFlag, err)
		}
		return e.fail("hum start: cannot resolve the project root: %v", err)
	}

	req := protocol.Request{
		Event: &protocol.Event{
			Event:     protocol.SessionStarted,
			ID:        id,
			Workspace: workspace,
			Title:     title,
			Root:      root,
			Priority:  priority,
			Metadata:  map[string]string(meta),
		},
	}

	code := send(e, req)
	if code == exitOK {
		fmt.Fprintln(e.stdout, id)
	}
	return code
}
