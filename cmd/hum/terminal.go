package main

import (
	"flag"
	"os"

	"github.com/mach4-braai/hum/internal/protocol"
)

func init() {
	register("complete", terminal("complete", protocol.SessionCompleted))
	register("fail", terminal("fail", protocol.SessionFailed))
	register("cancel", terminal("cancel", protocol.SessionCancelled))
}

func terminal(name string, event protocol.EventType) func(*env, []string) int {
	return func(e *env, words []string) int {
		return sendTerminal(e, name, event, words)
	}
}

func resolveSessionID(id string) string {
	if id != "" {
		return id
	}
	return os.Getenv(envSessionID)
}

const envSessionID = "HUM_SESSION_ID"

func sendTerminal(e *env, name string, event protocol.EventType, words []string) int {
	var id string

	rest, ok := operands(e, name, words, func(f *flag.FlagSet) {
		f.StringVar(&id, "id", id, "session id")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, name, rest[0])
	}

	id = resolveSessionID(id)
	if id == "" {
		return e.usagef("hum %s: no session id; pass --id or set %s", name, envSessionID)
	}

	return send(e, protocol.Request{Event: &protocol.Event{Event: event, ID: id}})
}
