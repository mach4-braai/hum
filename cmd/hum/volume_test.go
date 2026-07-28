package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/config"
	"github.com/mach4-braai/hum/internal/paths"
	"github.com/mach4-braai/hum/internal/protocol"
)

func volumeOKResponse() string {
	return `{"ok":true}` + "\n"
}

func volumeErrResponse() string {
	return `{"ok":false,"error":"hardware fault"}` + "\n"
}

func TestVolumeMuteSendsCommandAndPersists(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, volumeOKResponse())
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	req := await()
	if req.Command != protocol.CmdMute {
		t.Errorf("command = %q, want %q", req.Command, protocol.CmdMute)
	}

	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg == nil || !cfg.Audio.Muted {
		t.Errorf("audio.muted not written as true in config")
	}
}

func TestVolumeUnmuteSendsCommandAndPersists(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, volumeOKResponse())
	var stdout, stderr bytes.Buffer

	code := run([]string{"unmute"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	req := await()
	if req.Command != protocol.CmdUnmute {
		t.Errorf("command = %q, want %q", req.Command, protocol.CmdUnmute)
	}

	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg == nil || cfg.Audio.Muted {
		t.Errorf("audio.muted not written as false in config")
	}
}

func TestVolumeSetPreservesUnrelatedConfigKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)

	cfgPath := filepath.Join(home, paths.ConfigFileName)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cfgPath, []byte("project:\n  name: myproject\naudio:\n  volume: 0.6\n"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	await := serveOne(t, volumeOKResponse())
	var stdout, stderr bytes.Buffer

	code := run([]string{"volume", "0.4"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}
	_ = await()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg == nil {
		t.Fatal("config is nil after volume set")
	}
	if cfg.Audio.Volume != 0.4 {
		t.Errorf("audio.volume = %v, want 0.4", cfg.Audio.Volume)
	}
	if cfg.Project.Name != "myproject" {
		t.Errorf("project.name = %q, want %q", cfg.Project.Name, "myproject")
	}
}

func TestVolumeMuteToggleFromMutedSendsUnmute(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusResp := `{"ok":true,"data":{"sessions":[],"theme":"minimal","root":"D","scale":"minor_pentatonic","renderer":"nop","sample_rate":44100,"version":"dev","volume":0.6,"muted":true,"sounding_voices":0}}` + "\n"
	await := serveResponses(t, statusResp, volumeOKResponse())
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute", "--toggle"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}

	requests := await()
	if len(requests) < 2 {
		t.Fatalf("got %d requests, want 2", len(requests))
	}
	if requests[0].Command != protocol.CmdStatus {
		t.Errorf("requests[0].Command = %q, want %q", requests[0].Command, protocol.CmdStatus)
	}
	if requests[1].Command != protocol.CmdUnmute {
		t.Errorf("requests[1].Command = %q, want %q (toggle from muted should unmute)", requests[1].Command, protocol.CmdUnmute)
	}

	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg == nil || cfg.Audio.Muted {
		t.Errorf("audio.muted should be false after toggle from muted=true")
	}
}

func TestVolumeMuteToggleFromUnmutedSendsMute(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusResp := `{"ok":true,"data":{"sessions":[],"theme":"minimal","root":"D","scale":"minor_pentatonic","renderer":"nop","sample_rate":44100,"version":"dev","volume":0.6,"muted":false,"sounding_voices":0}}` + "\n"
	await := serveResponses(t, statusResp, volumeOKResponse())
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute", "--toggle"}, &stdout, &stderr)
	if code != exitOK {
		t.Fatalf("exit %d, stderr=%q", code, stderr.String())
	}

	requests := await()
	if len(requests) < 2 {
		t.Fatalf("got %d requests, want 2", len(requests))
	}
	if requests[1].Command != protocol.CmdMute {
		t.Errorf("requests[1].Command = %q, want %q (toggle from unmuted should mute)", requests[1].Command, protocol.CmdMute)
	}

	cfg, err := config.Load(paths.GlobalConfigFile())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg == nil || !cfg.Audio.Muted {
		t.Errorf("audio.muted should be true after toggle from muted=false")
	}
}

func TestVolumePrintShowsLevelAndMutedState(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())

	mutedResp := `{"ok":true,"data":{"sessions":[],"theme":"minimal","root":"D","scale":"minor_pentatonic","renderer":"nop","sample_rate":44100,"version":"dev","volume":0,"muted":true,"sounding_voices":0}}` + "\n"
	unmutedResp := `{"ok":true,"data":{"sessions":[],"theme":"minimal","root":"D","scale":"minor_pentatonic","renderer":"nop","sample_rate":44100,"version":"dev","volume":0,"muted":false,"sounding_voices":0}}` + "\n"

	awaitMuted := serveOne(t, mutedResp)
	var stdoutMuted, stderrMuted bytes.Buffer
	if code := run([]string{"volume"}, &stdoutMuted, &stderrMuted); code != exitOK {
		t.Fatalf("muted volume print: exit %d, stderr=%q", code, stderrMuted.String())
	}
	_ = awaitMuted()
	mutedOut := stdoutMuted.String()

	awaitUnmuted := serveOne(t, unmutedResp)
	var stdoutUnmuted, stderrUnmuted bytes.Buffer
	if code := run([]string{"volume"}, &stdoutUnmuted, &stderrUnmuted); code != exitOK {
		t.Fatalf("unmuted volume print: exit %d, stderr=%q", code, stderrUnmuted.String())
	}
	_ = awaitUnmuted()
	unmutedOut := stdoutUnmuted.String()

	if mutedOut == unmutedOut {
		t.Errorf("muted output %q == unmuted output %q, want them to differ", mutedOut, unmutedOut)
	}
	if !strings.Contains(mutedOut, "0.00") {
		t.Errorf("muted output %q does not contain the level %q", mutedOut, "0.00")
	}
	if !strings.Contains(unmutedOut, "0.00") {
		t.Errorf("unmuted output %q does not contain the level %q", unmutedOut, "0.00")
	}
	if !strings.Contains(mutedOut, "muted") {
		t.Errorf("muted output %q does not indicate muted state", mutedOut)
	}
	if strings.Contains(unmutedOut, "muted") {
		t.Errorf("unmuted output %q should not indicate muted state", unmutedOut)
	}
}

func TestVolumeSetRejectsOutOfRange(t *testing.T) {
	cases := []string{"1.5", "-0.1", "NaN", "loud"}
	for _, arg := range cases {
		t.Run(arg, func(t *testing.T) {
			t.Setenv("HUM_HOME", t.TempDir())
			await := serveResponses(t)
			var stdout, stderr bytes.Buffer

			code := run([]string{"volume", arg}, &stdout, &stderr)
			if code != exitUsage {
				t.Errorf("volume %q: exit %d, want %d", arg, code, exitUsage)
			}
			requests := await()
			if len(requests) != 0 {
				t.Errorf("volume %q: fake server received %d requests, want 0", arg, len(requests))
			}
		})
	}
}

func TestVolumeMuteDaemonRejectionExitsOneAndLeavesConfigUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HUM_HOME", home)
	await := serveOne(t, volumeErrResponse())

	cfgPath := paths.GlobalConfigFile()
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute"}, &stdout, &stderr)
	if code != exitDaemonError {
		t.Errorf("exit %d, want %d (daemon error)", code, exitDaemonError)
	}
	_ = await()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg != nil && cfg.Audio.Muted {
		t.Errorf("config was written despite daemon error")
	}
}

func TestVolumeMuteAbsentDaemonExitsThree(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("mute absent daemon: exit %d, want %d", code, exitUnreachable)
	}
}

func TestVolumeUnmuteAbsentDaemonExitsThree(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"unmute"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("unmute absent daemon: exit %d, want %d", code, exitUnreachable)
	}
}

func TestVolumeSetAbsentDaemonExitsThree(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"volume", "0.5"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("volume absent daemon: exit %d, want %d", code, exitUnreachable)
	}
}

func TestVolumePrintAbsentDaemonExitsThree(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"volume"}, &stdout, &stderr)
	if code != exitUnreachable {
		t.Errorf("volume (no args) absent daemon: exit %d, want %d", code, exitUnreachable)
	}
}

func TestVolumeMuteUnexpectedOperandIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute", "extra"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("mute with operand: exit %d, want %d", code, exitUsage)
	}
}

func TestVolumeUnmuteUnexpectedOperandIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"unmute", "extra"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("unmute with operand: exit %d, want %d", code, exitUsage)
	}
}

func TestVolumeUnmuteToggleFlagIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"unmute", "--toggle"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("unmute --toggle: exit %d, want %d", code, exitUsage)
	}
}

func TestVolumeSetMultipleOperandsIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"volume", "0.3", "0.4"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("volume with two operands: exit %d, want %d", code, exitUsage)
	}
}

func TestVolumeMuteSurvivesDaemonRestart(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	d := startHumd(t)

	socket := os.Getenv("HUM_SOCKET")
	code, out := runBinary(t, socket, "mute")
	if code != exitOK {
		t.Fatalf("mute: exit %d\n%s", code, out)
	}

	d.restart()

	e := &env{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		opts:   &options{timeout: defaultTimeout},
	}
	status, _, statusCode := fetchStatus(e)
	if statusCode != exitOK {
		t.Fatalf("fetchStatus after restart: exit %d\ndaemon logs:\n%s", statusCode, d.logs())
	}
	if !status.Muted {
		t.Errorf("daemon restarted but muted=false, want true\ndaemon logs:\n%s", d.logs())
	}
}
func TestVolumeMuteUnparsableFlagIsUsageError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute", "--bogus"}, &stdout, &stderr)

	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stderr=%q", code, exitUsage, stderr.String())
	}
	if stderr.String() == "" {
		t.Error("stderr is empty, want a usage error message")
	}
}

func TestVolumeMuteToggleFailsWhenStatusUnreachable(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	unreachableSocket(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute", "--toggle"}, &stdout, &stderr)

	if code != exitUnreachable {
		t.Fatalf("exit %d, want %d (unreachable); stderr=%q", code, exitUnreachable, stderr.String())
	}
	cfgPath := paths.GlobalConfigFile()
	if _, err := os.Stat(cfgPath); err == nil {
		t.Error("config file was written, but should not have been when status fetch fails")
	}
}

func TestVolumeMuteToggleFailsWhenStatusReturnsError(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	await := serveOne(t, `{"ok":false,"error":"sensor malfunction"}`+"\n")
	var stdout, stderr bytes.Buffer

	code := run([]string{"mute", "--toggle"}, &stdout, &stderr)
	req := await()

	if code != exitDaemonError {
		t.Fatalf("exit %d, want %d (daemon error); stderr=%q", code, exitDaemonError, stderr.String())
	}
	if req.Command != protocol.CmdStatus {
		t.Errorf("command = %q, want %q; no mute/unmute should be sent when status fails", req.Command, protocol.CmdStatus)
	}
	cfgPath := paths.GlobalConfigFile()
	if _, err := os.Stat(cfgPath); err == nil {
		t.Error("config file was written, but should not have been when status fetch fails")
	}
}

func TestVolumeJSONPrintsRawStatusPayload(t *testing.T) {
	t.Setenv("HUM_HOME", t.TempDir())
	statusResp := `{"ok":true,"data":{"sessions":[],"theme":"minimal","root":"D","scale":"minor_pentatonic","renderer":"nop","sample_rate":44100,"version":"dev","volume":0.42,"muted":false,"sounding_voices":0}}` + "\n"
	await := serveOne(t, statusResp)
	var stdout, stderr bytes.Buffer

	code := run([]string{"volume", "--json"}, &stdout, &stderr)
	_ = await()

	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr=%q", code, exitOK, stderr.String())
	}
	var payload protocol.StatusPayload
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &payload); err != nil {
		t.Fatalf("stdout is not valid StatusPayload JSON: %v; stdout=%q", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "(muted)") {
		t.Errorf("stdout=%q; --json must not include human-readable '(muted)' decoration", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("stderr=%q, want empty", stderr.String())
	}
}
