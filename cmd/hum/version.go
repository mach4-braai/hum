package main

import "github.com/mach4-braai/hum/internal/buildinfo"

func init() {
	register("version", runVersion)
}

func build() buildinfo.Info {
	return buildinfo.Resolve("hum", version, commit, date)
}

func runVersion(e *env, words []string) int {
	rest, ok := operands(e, "version", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "version", rest[0])
	}
	if err := build().Write(e.stdout, e.opts.asJSON); err != nil {
		return e.fail("hum: %v", err)
	}
	return exitOK
}
