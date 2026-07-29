package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
	"github.com/mach4-braai/hum/internal/theme"
)

func init() {
	register("doctor", runDoctor)
}

const nopRenderer = "nop"

type doctorCheck struct {
	Status string `json:"status"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

func runDoctor(e *env, words []string) int {
	var audioTest bool
	rest, ok := operands(e, "doctor", words, func(f *flag.FlagSet) {
		f.BoolVar(&audioTest, "audio-test", audioTest, "send a two-second audio test tone")
	})
	if !ok {
		return exitUsage
	}
	if len(rest) != 0 {
		return unexpected(e, "doctor", rest[0])
	}

	checks := doctorCollect(e, audioTest)

	if e.opts.asJSON {
		doctorPrintJSON(e, checks)
	} else {
		doctorPrintTable(e, checks)
	}

	for _, c := range checks {
		if c.Status == "fail" {
			return exitDaemonError
		}
	}
	return exitOK
}

func doctorCollect(e *env, audioTest bool) []doctorCheck {
	var checks []doctorCheck

	clientVersion := build().Version
	checks = append(checks, doctorCheck{"pass", "client", clientVersion})

	pingOK, pingDetail := doctorPingDaemon(e)
	if pingOK {
		checks = append(checks, doctorCheck{"pass", "daemon", pingDetail})
	} else {
		checks = append(checks, doctorCheck{"fail", "daemon", pingDetail})
	}
	checks = append(checks, doctorSupervisorCheck(pingOK))

	var st protocol.StatusPayload
	var statusOK bool
	if pingOK {
		st, statusOK = doctorFetchStatus(e)
	}

	if !pingOK {
		checks = append(checks, doctorCheck{"warn", "versions", "unknown (no daemon)"})
	} else if !statusOK {
		checks = append(checks, doctorCheck{"warn", "versions", "could not fetch daemon status"})
	} else if clientVersion == st.Version {
		checks = append(checks, doctorCheck{"pass", "versions", clientVersion})
	} else {
		checks = append(checks, doctorCheck{"warn", "versions", fmt.Sprintf("client %s  daemon %s", clientVersion, st.Version)})
	}

	checks = append(checks, doctorSocketCheck())

	cfg, prov, err := config.Resolve(nil, "")
	if err != nil {
		checks = append(checks, doctorCheck{"fail", "config", err.Error()})
	} else {
		checks = append(checks, doctorConfigChecks(cfg, prov)...)
	}

	themeName := ""
	if statusOK {
		themeName = st.Theme
	} else if cfg != nil {
		themeName = cfg.Music.Theme
	}
	checks = append(checks, doctorThemeCheck(themeName))

	if cfg != nil {
		checks = append(checks, doctorMusicCheck(cfg))
	}

	if !pingOK || !statusOK {
		checks = append(checks, doctorCheck{"warn", "audio", "unknown (no daemon)"})
	} else {
		checks = append(checks, doctorAudioCheck(st))
	}

	if audioTest {
		checks = append(checks, doctorRunAudioTest(e, pingOK))
	}

	return checks
}

func doctorPingDaemon(e *env) (bool, string) {
	got, err := query(protocol.Request{Command: protocol.CmdPing}, e.opts.timeout)
	if err != nil {
		if errors.Is(err, errNoDaemon) {
			return false, "no daemon listening"
		}
		return false, err.Error()
	}
	if !got.response.OK {
		return false, responseError(got.response)
	}
	return true, "responding"
}

func doctorFetchStatus(e *env) (protocol.StatusPayload, bool) {
	got, err := query(protocol.Request{Command: protocol.CmdStatus}, e.opts.timeout)
	if err != nil || !got.response.OK {
		return protocol.StatusPayload{}, false
	}
	var st protocol.StatusPayload
	if err := json.Unmarshal(got.response.Data, &st); err != nil {
		return protocol.StatusPayload{}, false
	}
	return st, true
}

func doctorSocketCheck() doctorCheck {
	p := paths.SocketPath()
	info, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return doctorCheck{"fail", "socket", p + ": not found"}
		}
		return doctorCheck{"warn", "socket", fmt.Sprintf("%s: %v", p, err)}
	}
	m := info.Mode()
	if m&os.ModeSocket == 0 {
		return doctorCheck{"fail", "socket", fmt.Sprintf("%s: not a socket (mode %s)", p, m)}
	}
	return doctorCheck{"pass", "socket", fmt.Sprintf("%s  perm=%04o", p, m.Perm())}
}

func doctorConfigChecks(cfg *config.Config, prov config.Provenance) []doctorCheck {
	type field struct {
		key string
		val string
	}
	fields := []field{
		{"project.name", cfg.Project.Name},
		{"music.root", cfg.Music.Root},
		{"music.scale", cfg.Music.Scale},
		{"music.theme", cfg.Music.Theme},
		{"audio.volume", fmt.Sprintf("%g", cfg.Audio.Volume)},
		{"audio.muted", fmt.Sprintf("%t", cfg.Audio.Muted)},
	}
	out := make([]doctorCheck, len(fields))
	for i, f := range fields {
		value := f.val
		if value == "" {
			value = "(unset)"
		}
		out[i] = doctorCheck{"pass", f.key, fmt.Sprintf("%s  (layer: %s)", value, prov[f.key])}
	}
	return out
}

func doctorThemeCheck(name string) doctorCheck {
	if name == "" {
		return doctorCheck{"warn", "theme", "unknown (no config)"}
	}
	userPath := filepath.Join(paths.GlobalConfigDir(), "themes", name+".yaml")
	if _, err := theme.Load(name); err != nil {
		return doctorCheck{"fail", "theme", fmt.Sprintf("%s: %v", name, err)}
	}
	if _, statErr := os.Stat(userPath); statErr == nil {
		return doctorCheck{"pass", "theme", fmt.Sprintf("%s  (user: %s)", name, userPath)}
	}
	return doctorCheck{"pass", "theme", name + "  (embedded)"}
}

func doctorMusicCheck(cfg *config.Config) doctorCheck {
	if _, err := harmony.ParseNoteClass(cfg.Music.Root); err != nil {
		return doctorCheck{"fail", "music", fmt.Sprintf("root %q: %v", cfg.Music.Root, err)}
	}
	if _, err := harmony.LookupScale(cfg.Music.Scale); err != nil {
		return doctorCheck{"fail", "music", fmt.Sprintf("scale %q: %v", cfg.Music.Scale, err)}
	}
	return doctorCheck{"pass", "music", fmt.Sprintf("root %s, scale %s", cfg.Music.Root, cfg.Music.Scale)}
}

func doctorAudioCheck(st protocol.StatusPayload) doctorCheck {
	detail := fmt.Sprintf("renderer=%s  sample_rate=%d", st.Renderer, st.SampleRate)
	if st.Renderer != nopRenderer {
		return doctorCheck{"pass", "audio", detail}
	}
	if st.RendererRequested == nopRenderer {
		return doctorCheck{"warn", "audio", detail + "  (headless by request)"}
	}
	if st.RendererRequested == "" {
		return doctorCheck{"warn", "audio", detail + "  (silent; the daemon did not say what it asked for)"}
	}
	return doctorCheck{"warn", "audio", fmt.Sprintf("%s  (fell back from %s: no audio device)", detail, st.RendererRequested)}
}

func doctorRunAudioTest(e *env, daemonOK bool) doctorCheck {
	if !daemonOK {
		return doctorCheck{"fail", "audio-test", "no daemon listening"}
	}
	got, err := query(protocol.Request{Command: protocol.CmdAudioTest}, e.opts.timeout)
	if err != nil {
		return doctorCheck{"fail", "audio-test", err.Error()}
	}
	if !got.response.OK {
		return doctorCheck{"fail", "audio-test", responseError(got.response)}
	}
	var payload protocol.AudioTestPayload
	if err := json.Unmarshal(got.response.Data, &payload); err != nil {
		return doctorCheck{"fail", "audio-test", fmt.Sprintf("malformed response: %v", err)}
	}
	if !payload.Played {
		if payload.Renderer == nopRenderer {
			return doctorCheck{"warn", "audio-test", "no-op: the nop renderer produces no sound"}
		}
		if payload.Muted {
			return doctorCheck{"warn", "audio-test", "no-op: the daemon is muted"}
		}
		return doctorCheck{"warn", "audio-test", "no-op: the tone was not played"}
	}
	return doctorCheck{"pass", "audio-test", fmt.Sprintf("tone played (%.1fs, renderer=%s)", payload.Seconds, payload.Renderer)}
}

func doctorPrintTable(e *env, checks []doctorCheck) {
	tw := tabwriter.NewWriter(e.stdout, 0, 0, 2, ' ', 0)
	for _, c := range checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Status, c.Name, c.Detail)
	}
	tw.Flush()
}

func doctorPrintJSON(e *env, checks []doctorCheck) {
	data, _ := json.Marshal(checks)
	fmt.Fprintln(e.stdout, string(data))
}
