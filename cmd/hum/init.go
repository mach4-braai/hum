package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/theme"
)

var osGetwd = os.Getwd

func init() {
	register("init", runInit)
}

func runInit(e *env, words []string) int {
	var initForce, initGlobal, initPrint bool
	rest, ok := operands(e, "init", words, func(f *flag.FlagSet) {
		f.BoolVar(&initForce, "force", initForce, "overwrite an existing configuration")
		f.BoolVar(&initGlobal, "global", initGlobal, "write the global configuration instead")
		f.BoolVar(&initPrint, "print", initPrint, "print the configuration without writing it")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) > 0 {
		return unexpected(e, "init", rest[0])
	}

	wd, err := osGetwd()
	if err != nil {
		return e.fail("hum init: %v", err)
	}

	targetPath := paths.GlobalConfigFile()
	if !initGlobal {
		targetPath = filepath.Join(wd, paths.ProjectDirName, paths.ConfigFileName)
	}

	doc := initDocument(initProjectName(wd))

	if initPrint {
		fmt.Fprint(e.stdout, doc)
		return exitOK
	}

	if !initForce {
		if _, err := os.Stat(targetPath); err == nil {
			return e.fail("hum init: %s: already exists (use --force to overwrite)", targetPath)
		}
	}

	if err := config.Write(targetPath, []byte(doc)); err != nil {
		return e.fail("hum init: %s: %v", targetPath, err)
	}
	fmt.Fprintln(e.stdout, targetPath)
	return exitOK
}

func initProjectName(startDir string) string {
	if root, err := paths.ProjectRoot(startDir); err == nil {
		return filepath.Base(root)
	}
	return filepath.Base(startDir)
}

func initDocument(name string) string {
	d := config.Default()
	var b strings.Builder
	b.WriteString("project:\n")
	fmt.Fprintf(&b, "  name: %q\n", name)
	b.WriteString("\n")
	b.WriteString("music:\n")
	fmt.Fprintf(&b, "  root: %s\n", d.Music.Root)
	fmt.Fprintf(&b, "  octave: %d\n", d.Music.Octave)
	fmt.Fprintf(&b, "  # the drone root sounds here; %d is lowest, %d highest\n", config.MinOctave, config.MaxOctave)
	fmt.Fprintf(&b, "  scale: %s\n", d.Music.Scale)
	fmt.Fprintf(&b, "  # valid scales: %s\n", strings.Join(harmony.ScaleNames(), ", "))
	fmt.Fprintf(&b, "  theme: %s\n", d.Music.Theme)
	fmt.Fprintf(&b, "  # valid themes: %s\n", strings.Join(theme.List(), ", "))
	b.WriteString("\n")
	b.WriteString("audio:\n")
	fmt.Fprintf(&b, "  volume: %g\n", d.Audio.Volume)
	fmt.Fprintf(&b, "  muted: %v\n", d.Audio.Muted)
	return b.String()
}
