package main

import (
	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/paths"
)

func persist(e *env, values map[string]string) int {
	path := paths.GlobalConfigFile()
	if err := config.Patch(path, values); err != nil {
		return e.fail("hum: cannot update %s: %v", path, err)
	}
	return exitOK
}
