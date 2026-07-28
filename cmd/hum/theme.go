package main

import (
	"fmt"

	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/theme"
)

func init() {
	register("theme", runTheme)
}

func runTheme(e *env, words []string) int {
	rest, ok := operands(e, "theme", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) == 0 {
		return e.usagef("hum theme: expected \"list\" or \"use\"")
	}
	switch rest[0] {
	case "list":
		return themeList(e, rest[1:])
	case "use":
		return themeUse(e, rest[1:])
	default:
		return e.usagef("hum theme: unknown subcommand %q; want \"list\" or \"use\"", rest[0])
	}
}

func themeList(e *env, positionals []string) int {
	if len(positionals) != 0 {
		return unexpected(e, "theme list", positionals[0])
	}
	var p protocol.ThemeListPayload
	raw, code := fetchPayload(e, protocol.Request{Command: protocol.CmdThemeList}, &p)
	if code != exitOK {
		return code
	}
	if e.opts.asJSON {
		printJSON(e, raw)
		return exitOK
	}
	for _, name := range p.Themes {
		if name == p.Active {
			fmt.Fprintf(e.stdout, "* %s\n", name)
		} else {
			fmt.Fprintf(e.stdout, "  %s\n", name)
		}
	}
	return exitOK
}

func themeUse(e *env, positionals []string) int {
	if len(positionals) == 0 {
		return e.usagef("hum theme use: NAME is required")
	}
	if len(positionals) > 1 {
		return unexpected(e, "theme use", positionals[1])
	}
	name := positionals[0]
	if _, err := theme.Load(name); err != nil {
		return e.fail("hum theme use: %v", err)
	}
	var usePayload protocol.ThemeUsePayload
	raw, code := fetchPayload(e, protocol.Request{Command: protocol.CmdThemeUse, Value: name}, &usePayload)
	if code != exitOK {
		return code
	}
	if e.opts.asJSON {
		printJSON(e, raw)
	}
	return persist(e, map[string]string{"music.theme": name})
}
