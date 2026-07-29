package main

import (
	"flag"

	"github.com/mach4-braai/hum/internal/protocol"
)

func init() {
	register("update", runUpdate)
}

func runUpdate(e *env, words []string) int {
	var id, title string
	var meta metaFlag

	rest, ok := operands(e, "update", words, func(f *flag.FlagSet) {
		f.StringVar(&id, "id", id, "session id")
		f.StringVar(&title, "title", title, "session title")
		f.Var(&meta, "meta", "k=v metadata")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "update", rest[0])
	}

	id = resolveSessionID(id)
	if id == "" {
		return e.usagef("hum update: no session id; pass --id or set %s", envSessionID)
	}

	return send(e, protocol.Request{Event: &protocol.Event{
		Event:    protocol.SessionUpdated,
		ID:       id,
		Title:    title,
		Metadata: map[string]string(meta),
	}})
}
