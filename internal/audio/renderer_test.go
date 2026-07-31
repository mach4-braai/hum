package audio

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/mach4-braai/hum/internal/harmony"
	"github.com/mach4-braai/hum/internal/renderer"
	"github.com/mach4-braai/hum/internal/theme"
)

func testOpts() renderer.Options {
	return renderer.Options{
		SampleRate: 48000,
		Volume:     0.6,
		Theme: theme.Theme{
			Drone: theme.DroneSpec{
				Attack:  0.01,
				Release: 0.05,
				Gain:    0.7,
			},
			Phrases: theme.PhrasesSpec{
				Attack: 0.005,
				Decay:  0.05,
			},
		},
	}
}

func newTestRenderer(t *testing.T) *AudioRenderer {
	t.Helper()
	f := DefaultFormat()
	m := NewMixer(f)
	r := newRendererWithMixer(m, f, testOpts())
	r.rampDuration = 0
	return r
}

func voiceState(sessionID string, class, octave int) harmony.VoiceState {
	return harmony.VoiceState{
		Voice: harmony.Voice{
			SessionID: sessionID,
			Pitch:     harmony.Pitch{Class: class, Octave: octave},
		},
	}
}

func TestUpdate_AddVoiceCreatesOneOsc(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	if err := r.Update(state); err != nil {
		t.Fatal(err)
	}
	if got := r.mixer.Len(); got != 1 {
		t.Fatalf("want 1 osc, got %d", got)
	}
}

func TestUpdate_ReleaseNotDelete(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	r.Update(state)

	r.Update(harmony.State{})

	if got := r.mixer.Len(); got != 1 {
		t.Fatalf("released voice should still be in mixer, got Len=%d", got)
	}

	buf := make([]byte, DefaultFormat().SampleRate*8)
	for range 100 {
		r.mixer.Read(buf)
		if r.mixer.Len() == 0 {
			return
		}
	}
	t.Fatal("released voice never removed from mixer after draining samples")
}

func TestUpdate_Idempotent(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}

	r.Update(state)
	if r.mixer.Len() != 1 {
		t.Fatal("want 1 after first Update")
	}

	r.Update(state)
	if r.mixer.Len() != 1 {
		t.Fatal("idempotent Update must not create a second osc")
	}
}

func TestUpdate_Idempotent_NoGainReset(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	r.Update(state)

	buf := make([]byte, 512)
	r.mixer.Read(buf)

	gainBefore := r.mixer.Gain()
	r.Update(state)
	gainAfter := r.mixer.Gain()

	if gainBefore != gainAfter {
		t.Fatalf("idempotent Update must not change mixer gain: before=%v after=%v", gainBefore, gainAfter)
	}
}

func TestSetVolume_NaN(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.SetVolume(math.NaN()); err == nil {
		t.Fatal("SetVolume(NaN) must return an error")
	}
}

func TestSetVolume_OutOfRange(t *testing.T) {
	r := newTestRenderer(t)
	for _, bad := range []float64{-0.1, 1.1, math.Inf(1), math.Inf(-1)} {
		if err := r.SetVolume(bad); err == nil {
			t.Fatalf("SetVolume(%v) must return an error", bad)
		}
	}
}

func TestSetVolume_Valid(t *testing.T) {
	r := newTestRenderer(t)
	for _, v := range []float64{0, 0.5, 1} {
		if err := r.SetVolume(v); err != nil {
			t.Fatalf("SetVolume(%v) unexpected error: %v", v, err)
		}
	}
}

func TestSetMuted_RestoresVolume(t *testing.T) {
	r := newTestRenderer(t)
	r.SetVolume(0.6)

	r.SetMuted(true)
	r.SetMuted(false)

	if got := r.mixer.Gain(); got != 0.6 {
		t.Fatalf("want gain 0.6 after unmute, got %v", got)
	}
}

func TestSetMuted_Idempotent(t *testing.T) {
	r := newTestRenderer(t)
	r.SetMuted(true)
	r.SetMuted(true)
	r.SetMuted(false)
	r.SetMuted(false)
}

func TestClose_Safe(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClose_UpdateAfterClose(t *testing.T) {
	r := newTestRenderer(t)
	r.Close()
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	if err := r.Update(state); err != nil {
		t.Fatal(err)
	}
	if r.mixer.Len() != 0 {
		t.Fatal("Update after Close must not add sources")
	}
}

func TestName(t *testing.T) {
	r := newTestRenderer(t)
	if r.Name() != "audio" {
		t.Fatalf("want name audio, got %q", r.Name())
	}
}

func TestTrigger_PhraseCap(t *testing.T) {
	r := newTestRenderer(t)
	phrase := harmony.Phrase{
		Notes: []harmony.Note{
			{
				Pitch:    harmony.Pitch{Class: 9, Octave: 4},
				Offset:   0,
				Duration: 100e6,
				Gain:     0.5,
			},
		},
	}
	for range 100 {
		r.Trigger(phrase)
	}
	if got := r.mixer.Len(); got > maxPhraseVoices {
		t.Fatalf("phrase voices %d exceed cap %d", got, maxPhraseVoices)
	}
}

func TestTrigger_AfterClose(t *testing.T) {
	r := newTestRenderer(t)
	r.Close()
	err := r.Trigger(harmony.Phrase{Notes: []harmony.Note{{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Duration: 100e6,
		Gain:     0.5,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if r.mixer.Len() != 0 {
		t.Fatal("Trigger after Close must not add sources")
	}
}

func TestRegistered(t *testing.T) {
	names := renderer.Names()
	found := false
	for _, n := range names {
		if n == "audio" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("audio renderer not registered")
	}
}

func TestNewRendererWithMixer_Muted(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	opts := testOpts()
	opts.Muted = true
	r := newRendererWithMixer(m, f, opts)
	if got := m.Gain(); got != 0 {
		t.Fatalf("muted renderer: gain = %v, want 0", got)
	}
	_ = r
}

func TestSetTheme_UpdatesActiveVoices(t *testing.T) {
	r := newTestRenderer(t)
	state := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	if err := r.Update(state); err != nil {
		t.Fatal(err)
	}
	newTheme := testOpts().Theme
	newTheme.Drone.Gain = 0.3
	if err := r.SetTheme(newTheme); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	if r.th.Drone.Gain != 0.3 {
		t.Errorf("th.Drone.Gain = %v, want 0.3", r.th.Drone.Gain)
	}
}

func TestSetTheme_RetargetsTheEnvelopeWithoutRestartingTheAttack(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.Update(harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}); err != nil {
		t.Fatal(err)
	}
	osc := r.active["s1"].osc

	buf := make([][2]float32, 256)
	osc.Mix(buf)
	gainBefore, posBefore := osc.curGain, osc.envPos
	if osc.state != envAttack {
		t.Fatalf("state = %v after one buffer, want the voice still attacking", osc.state)
	}

	swapped := testOpts().Theme
	swapped.Drone.Attack = 8
	swapped.Drone.Release = 6
	if err := r.SetTheme(swapped); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	wantRelease := 6 * float64(r.format.SampleRate)
	if osc.releaseSamples != wantRelease {
		t.Errorf("releaseSamples = %v after the swap, want %v from the new theme", osc.releaseSamples, wantRelease)
	}
	if want := 8 * float64(r.format.SampleRate); osc.attackSamples != want {
		t.Errorf("attackSamples = %v after the swap, want %v from the new theme", osc.attackSamples, want)
	}
	if osc.state != envAttack {
		t.Errorf("state = %v after the swap, want the voice still attacking rather than retriggered", osc.state)
	}
	if osc.envPos != posBefore {
		t.Errorf("envPos = %v after the swap, want %v: restarting it replays the attack", osc.envPos, posBefore)
	}
	if osc.curGain != gainBefore {
		t.Errorf("curGain = %v after the swap, want %v: a jump back is an audible click", osc.curGain, gainBefore)
	}
}

func TestSetEnvelope_ShorteningTheAttackEndsItWithoutAJump(t *testing.T) {
	osc := NewOsc(DefaultFormat(), 440, 0.5, Envelope{Attack: time.Second, Release: time.Second})
	osc.Mix(make([][2]float32, 64))
	before := osc.curGain

	osc.SetEnvelope(Envelope{Attack: time.Microsecond, Release: time.Second})
	osc.Mix(make([][2]float32, 1))

	if osc.state != envSustain {
		t.Errorf("state = %v, want sustain once the shortened attack is already past", osc.state)
	}
	if osc.curGain < before {
		t.Errorf("curGain = %v, want it to keep rising from %v rather than fall back", osc.curGain, before)
	}
	if jump := osc.curGain - before; jump > 0.01 {
		t.Errorf("curGain jumped by %v in one sample, want a continuous level: snapping to the peak is an audible click", jump)
	}
}

func TestSetEnvelope_LengtheningTheAttackDoesNotOvershoot(t *testing.T) {
	const gain = 0.5
	osc := NewOsc(DefaultFormat(), 440, gain, Envelope{Attack: 10 * time.Millisecond, Release: time.Second})
	buf := make([][2]float32, 256)
	osc.Mix(buf)

	osc.SetEnvelope(Envelope{Attack: 20 * time.Millisecond, Release: time.Second})
	for osc.state == envAttack {
		osc.Mix(buf)
		if osc.curGain > gain+0.0001 {
			t.Fatalf("curGain = %v during a lengthened attack, want it never above the peak %v", osc.curGain, gain)
		}
	}

	if osc.curGain != gain {
		t.Errorf("curGain = %v once the attack ends, want the peak %v", osc.curGain, gain)
	}
}

func TestSetTheme_MidAttackSwapApproachesTheNewGain(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.Update(harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}); err != nil {
		t.Fatal(err)
	}
	osc := r.active["s1"].osc
	osc.Mix(make([][2]float32, 256))

	const quieter = 0.2
	swapped := testOpts().Theme
	swapped.Drone.Gain = quieter
	swapped.Drone.Attack = 0.01
	if err := r.SetTheme(swapped); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	for osc.state == envAttack {
		osc.Mix(make([][2]float32, 64))
	}

	if osc.curGain != quieter {
		t.Errorf("curGain = %v once the retargeted attack ends, want the new theme's %v rather than the old peak", osc.curGain, quieter)
	}
}

func TestSetEnvelope_ZeroAttackDoesNotProduceInfiniteGain(t *testing.T) {
	osc := NewOsc(DefaultFormat(), 440, 0.5, Envelope{Attack: time.Second, Release: time.Second})
	buf := make([][2]float32, 64)
	osc.Mix(buf)

	osc.SetEnvelope(Envelope{Release: time.Second})
	for i := range buf {
		buf[i] = [2]float32{}
	}
	osc.Mix(buf)

	for i, frame := range buf {
		for c, sample := range frame {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				t.Fatalf("sample %d channel %d = %v, want a finite value; dividing by a zero-length attack ruins the buffer", i, c, sample)
			}
		}
	}
}

func TestSetTheme_NoActiveVoices(t *testing.T) {
	r := newTestRenderer(t)
	newTheme := testOpts().Theme
	if err := r.SetTheme(newTheme); err != nil {
		t.Fatalf("SetTheme with no voices: %v", err)
	}
}

func TestUpdate_RetargetExpression(t *testing.T) {
	r := newTestRenderer(t)
	state1 := harmony.State{Voices: []harmony.VoiceState{voiceState("s1", 9, 4)}}
	if err := r.Update(state1); err != nil {
		t.Fatal(err)
	}
	state2 := harmony.State{Voices: []harmony.VoiceState{
		{
			Voice:      harmony.Voice{SessionID: "s1", Pitch: harmony.Pitch{Class: 9, Octave: 4}},
			Expression: harmony.Expression{Intensity: 0.5},
		},
	}}
	if err := r.Update(state2); err != nil {
		t.Fatal(err)
	}
	if r.mixer.Len() != 1 {
		t.Fatalf("retarget Update must not create new osc; Len=%d", r.mixer.Len())
	}
}

func TestDroneEnvelope_Fallbacks(t *testing.T) {
	env := droneEnvelope(theme.DroneSpec{Attack: 0, Release: 0})
	if env.Attack != fallbackAttack {
		t.Errorf("attack fallback = %v, want %v", env.Attack, fallbackAttack)
	}
	if env.Release != fallbackRelease {
		t.Errorf("release fallback = %v, want %v", env.Release, fallbackRelease)
	}
}

func TestDoRamp_EarlyExitOnStaleGen(t *testing.T) {
	r := newTestRenderer(t)
	r.mixer.SetGain(0.8)

	r.mu.Lock()
	r.rampGen = 42
	r.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		r.doRamp(0.8, 0, 0, 1)
	}()
	<-done

	if got := r.mixer.Gain(); got != 0.8 {
		t.Errorf("stale doRamp must not change gain: got %v, want 0.8", got)
	}
}

func TestDoRamp_NormalCompletion(t *testing.T) {
	r := newTestRenderer(t)
	r.mixer.SetGain(0.8)

	r.mu.Lock()
	r.rampGen = 7
	r.mu.Unlock()

	r.doRamp(0.8, 0, 0, 7)

	if got := r.mixer.Gain(); got > 0.05 {
		t.Errorf("doRamp completed: gain = %v, want near 0", got)
	}
}

func TestClose_WithEngine(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	r := newRendererWithMixer(m, f, testOpts())
	mp := &mockPlayer{}
	r.engine = &Engine{player: mp}

	if err := r.Close(); err != nil {
		t.Fatalf("Close with engine: %v", err)
	}
	if !mp.paused {
		t.Fatal("engine.Close must pause the player")
	}
}

func TestTrigger_ZeroNotes(t *testing.T) {
	r := newTestRenderer(t)
	if err := r.Trigger(harmony.Phrase{}); err != nil {
		t.Fatal(err)
	}
	if r.mixer.Len() != 0 {
		t.Fatal("empty phrase must not add sources")
	}
}

func TestTrigger_CapDropsOldest(t *testing.T) {
	r := newTestRenderer(t)
	longNote := harmony.Note{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Duration: 10 * time.Second,
		Gain:     0.5,
	}
	for range maxPhraseVoices {
		if err := r.Trigger(harmony.Phrase{Notes: []harmony.Note{longNote}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.phraseIDs) == 0 {
		t.Fatal("phraseIDs must not be empty")
	}
	firstID := r.phraseIDs[0]

	if err := r.Trigger(harmony.Phrase{Notes: []harmony.Note{longNote}}); err != nil {
		t.Fatal(err)
	}
	if r.mixer.Has(firstID) {
		t.Error("oldest phrase voice must be dropped when cap exceeded")
	}
}

func TestDroppedPhrasesCountsWhatTheCapDiscarded(t *testing.T) {
	r := newTestRenderer(t)
	longNote := harmony.Note{
		Pitch:    harmony.Pitch{Class: 9, Octave: 4},
		Duration: 10 * time.Second,
		Gain:     0.5,
	}
	for range maxPhraseVoices {
		if err := r.Trigger(harmony.Phrase{Notes: []harmony.Note{longNote}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.DroppedPhrases(); got != 0 {
		t.Fatalf("dropped = %d while still under the cap, want 0", got)
	}

	for range 3 {
		if err := r.Trigger(harmony.Phrase{Notes: []harmony.Note{longNote}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.DroppedPhrases(); got != 3 {
		t.Errorf("dropped = %d after three phrases over the cap, want 3: an operator cannot see phrase loss the mixer does not count", got)
	}
}

func TestSetVolume_WhileMuted(t *testing.T) {
	f := DefaultFormat()
	m := NewMixer(f)
	opts := testOpts()
	opts.Muted = true
	r := newRendererWithMixer(m, f, opts)
	r.rampDuration = 0

	if err := r.SetVolume(0.4); err != nil {
		t.Fatal(err)
	}
	if got := m.Gain(); got != 0 {
		t.Errorf("gain during mute after SetVolume = %v, want 0", got)
	}

	if err := r.SetMuted(false); err != nil {
		t.Fatal(err)
	}
	if got := m.Gain(); got != 0.4 {
		t.Errorf("gain after unmute = %v, want 0.4", got)
	}
}

func TestSetMuted_DoubleMuteUnmute(t *testing.T) {
	r := newTestRenderer(t)
	r.SetVolume(0.6)

	r.SetMuted(true)
	r.SetMuted(true)
	r.SetMuted(false)
	r.SetMuted(false)

	if got := r.mixer.Gain(); got != 0.6 {
		t.Errorf("double toggle: gain = %v, want 0.6", got)
	}
}

func TestRendererInit_ErrNoDevice(t *testing.T) {
	orig := newOtoContext
	t.Cleanup(func() { newOtoContext = orig })
	newOtoContext = func(_ *oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, nil, errors.New("stub: no device")
	}

	_, err := renderer.Open("audio", renderer.Options{SampleRate: 0})
	if err == nil {
		t.Fatal("expected error when no device")
	}
	if !errors.Is(err, ErrNoDevice) {
		t.Errorf("want ErrNoDevice in chain, got %v", err)
	}
}

func TestRendererInit_Success(t *testing.T) {
	orig1 := newOtoContext
	orig2 := newOtoPlayer
	t.Cleanup(func() { newOtoContext = orig1; newOtoPlayer = orig2 })

	ready := make(chan struct{})
	close(ready)
	newOtoContext = func(_ *oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, ready, nil
	}
	newOtoPlayer = func(_ *oto.Context, _ *Mixer) playerPauser {
		return &mockPlayer{}
	}

	r, err := renderer.Open("audio", renderer.Options{SampleRate: 48000, Volume: 0.5})
	if err != nil {
		t.Fatalf("renderer.Open: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewAudioRenderer_ZeroSampleRate(t *testing.T) {
	orig := newOtoContext
	t.Cleanup(func() { newOtoContext = orig })
	newOtoContext = func(_ *oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, nil, errors.New("stub: no device")
	}

	_, err := newAudioRenderer(renderer.Options{SampleRate: 0})
	if err == nil {
		t.Fatal("expected error from newAudioRenderer with no device")
	}
	if !errors.Is(err, ErrNoDevice) {
		t.Errorf("want ErrNoDevice, got %v", err)
	}
}

func TestSampleRate_ReportsTheOutputFormat(t *testing.T) {
	r := newTestRenderer(t)

	if got := r.SampleRate(); got != DefaultFormat().SampleRate {
		t.Fatalf("SampleRate() = %d, want %d", got, DefaultFormat().SampleRate)
	}
}

func TestNewCaptureRendererMixesWithoutADevice(t *testing.T) {
	f := DefaultFormat()
	r, m := NewCaptureRenderer(f, testOpts())
	t.Cleanup(func() { r.Close() })

	if m == nil {
		t.Fatal("no mixer returned; the caller has nothing to read")
	}
	if err := r.Update(harmony.State{Voices: []harmony.VoiceState{voiceState("one", 2, 3)}}); err != nil {
		t.Fatalf("update: %v", err)
	}

	buf := make([]byte, 512*frameSize)
	if _, err := m.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if m.Len() != 1 {
		t.Errorf("mixer holds %d sources, want the one drone", m.Len())
	}
}

func TestOpenAudioRendererUsesTheRequestedSampleRate(t *testing.T) {
	stubSeams(t, true)

	r, err := openAudioRenderer(renderer.Options{SampleRate: 22050})
	if err != nil {
		t.Fatalf("openAudioRenderer: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	if got := r.(*AudioRenderer).SampleRate(); got != 22050 {
		t.Errorf("SampleRate = %d, want the requested 22050", got)
	}
}

func TestOpenAudioRendererFallsBackToTheDefaultFormat(t *testing.T) {
	stubSeams(t, true)

	r, err := openAudioRenderer(renderer.Options{})
	if err != nil {
		t.Fatalf("openAudioRenderer: %v", err)
	}
	t.Cleanup(func() { r.Close() })

	if got, want := r.(*AudioRenderer).SampleRate(), DefaultFormat().SampleRate; got != want {
		t.Errorf("SampleRate = %d, want the default %d when none is requested", got, want)
	}
}

func TestOpenAudioRendererFailsWhenNoDeviceOpens(t *testing.T) {
	orig := newOtoContext
	t.Cleanup(func() { newOtoContext = orig })
	newOtoContext = func(*oto.NewContextOptions) (*oto.Context, chan struct{}, error) {
		return nil, nil, errors.New("stub: no device")
	}

	if _, err := openAudioRenderer(renderer.Options{}); err == nil {
		t.Error("openAudioRenderer = nil error, want the device failure surfaced rather than a silent renderer")
	}
}
