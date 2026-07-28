package main

import (
	"flag"
	"fmt"
	"strconv"

	"github.com/mach4-braai/hum/internal/protocol"
)

func init() {
	register("mute", runMute)
	register("unmute", runUnmute)
	register("volume", runVolume)
}

func runMute(e *env, words []string) int {
	var toggle bool
	rest, ok := operands(e, "mute", words, func(f *flag.FlagSet) {
		f.BoolVar(&toggle, "toggle", toggle, "mute when sounding, unmute when muted")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "mute", rest[0])
	}
	if !toggle {
		return applyMute(e, true)
	}

	status, _, code := fetchStatus(e)
	if code != exitOK {
		return code
	}
	return applyMute(e, !status.Muted)
}

func runUnmute(e *env, words []string) int {
	rest, ok := operands(e, "unmute", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "unmute", rest[0])
	}
	return applyMute(e, false)
}

func applyMute(e *env, muted bool) int {
	command := protocol.CmdUnmute
	if muted {
		command = protocol.CmdMute
	}
	if code := send(e, protocol.Request{Command: command}); code != exitOK {
		return code
	}
	return persist(e, map[string]string{"audio.muted": strconv.FormatBool(muted)})
}

func runVolume(e *env, words []string) int {
	rest, ok := operands(e, "volume", words, nil)
	if !ok {
		return exitUsage
	}
	if len(rest) == 0 {
		return volumePrint(e)
	}
	if len(rest) > 1 {
		return unexpected(e, "volume", rest[1])
	}
	return volumeSet(e, rest[0])
}

func volumePrint(e *env) int {
	status, raw, code := fetchStatus(e)
	if code != exitOK {
		return code
	}
	if e.opts.asJSON {
		printJSON(e, raw)
		return exitOK
	}
	if status.Muted {
		fmt.Fprintf(e.stdout, "%.2f (muted)\n", status.Volume)
	} else {
		fmt.Fprintf(e.stdout, "%.2f\n", status.Volume)
	}
	return exitOK
}

func volumeSet(e *env, arg string) int {
	v, err := strconv.ParseFloat(arg, 64)
	if err != nil || !(v >= 0 && v <= 1) {
		return e.usagef("hum volume: %q is not a valid volume (must be a number in [0.0, 1.0])", arg)
	}
	code := send(e, protocol.Request{Command: protocol.CmdVolume, Value: strconv.FormatFloat(v, 'f', -1, 64)})
	if code != exitOK {
		return code
	}
	return persist(e, map[string]string{"audio.volume": strconv.FormatFloat(v, 'f', -1, 64)})
}
