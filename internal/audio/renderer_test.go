package audio

import (
	"errors"
	"fmt"
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

func TestSetMutedFadesOnTheSampleClockNotAGoroutine(t *testing.T) {
	f := DefaultFormat()
	r, m := NewCaptureRenderer(f, testOpts())
	m.Add("a", DroneBus, &constSource{l: 0.5, r: 0.5})

	buf := make([]byte, 1024*frameSize)
	m.Read(buf)
	sounding := decodeFrames(buf)
	before := sounding[len(sounding)-1][0]

	if err := r.SetMuted(true); err != nil {
		t.Fatal(err)
	}
	m.Read(buf)
	fading := decodeFrames(buf)

	if fading[0][0] >= before {
		t.Fatalf("first frame after mute is %.6f, not below %.6f: the fade is waiting on a clock other than the sample clock",
			fading[0][0], before)
	}
	if last := fading[len(fading)-1][0]; last > before*0.7 {
		t.Fatalf("21 ms after mute the level is %.6f against %.6f: the fade is too slow to feel immediate",
			last, before)
	}
}

func TestZeroVolumeStartsSilent(t *testing.T) {
	f := DefaultFormat()
	opts := testOpts()
	opts.Volume = 0
	_, m := NewCaptureRenderer(f, opts)
	m.Add("a", DroneBus, &constSource{l: 0.5, r: 0.5})

	buf := make([]byte, 256*frameSize)
	m.Read(buf)
	for i, fr := range decodeFrames(buf) {
		if fr[0] != 0 || fr[1] != 0 {
			t.Fatalf("frame %d is L=%v R=%v with volume 0: a user who configured silence is hearing something",
				i, fr[0], fr[1])
		}
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

func renderSummed(t *testing.T, voices []harmony.VoiceState, notes []harmony.Note, frames int) []float64 {
	t.Helper()
	f := DefaultFormat()
	r, m := NewCaptureRenderer(f, testOpts())
	if len(voices) > 0 {
		if err := r.Update(harmony.State{Voices: voices}); err != nil {
			t.Fatal(err)
		}
	}
	if len(notes) > 0 {
		if err := r.Trigger(harmony.Phrase{Notes: notes}); err != nil {
			t.Fatal(err)
		}
	}
	buf := make([]byte, frames*frameSize)
	m.Read(buf)
	summed := make([]float64, frames)
	for i, fr := range decodeFrames(buf) {
		summed[i] = fr[0]
	}
	return summed
}

func assertDroneUnducked(t *testing.T, voices []harmony.VoiceState, notes []harmony.Note) {
	t.Helper()
	const frames = 4096
	alone := renderSummed(t, voices, nil, frames)
	chimeOnly := renderSummed(t, nil, notes, frames)
	together := renderSummed(t, voices, notes, frames)

	var loudest float64
	for _, v := range chimeOnly {
		if a := math.Abs(v); a > loudest {
			loudest = a
		}
	}
	if loudest < 1e-3 {
		t.Fatalf("the chime peaked at %.6f, so the comparison proves nothing", loudest)
	}

	for i := range alone {
		if got := together[i] - chimeOnly[i]; math.Abs(got-alone[i]) > 1e-4 {
			t.Fatalf("frame %d: the drone contributes %.6f while the chime sounds, against %.6f alone: the chime is ducking it",
				i, got, alone[i])
		}
	}
}

func chimeNote(class, octave int, gain float64) harmony.Note {
	return harmony.Note{
		Pitch:    harmony.Pitch{Class: class, Octave: octave},
		Duration: 300 * time.Millisecond,
		Gain:     gain,
	}
}

func TestCompletionChimeDoesNotDuckOneDrone(t *testing.T) {
	assertDroneUnducked(t,
		[]harmony.VoiceState{voiceState("s1", 0, 4)},
		[]harmony.Note{chimeNote(7, 5, 0.7)})
}

func TestFailureCadenceDoesNotDuckTwoDrones(t *testing.T) {
	assertDroneUnducked(t,
		[]harmony.VoiceState{voiceState("s1", 0, 4), voiceState("s2", 7, 4)},
		[]harmony.Note{chimeNote(3, 4, 0.35), chimeNote(1, 4, 0.35)})
}

func TestChimeLevelIsIndependentOfHowManyDronesSound(t *testing.T) {
	const frames = 4096
	notes := []harmony.Note{chimeNote(7, 5, 0.7)}

	var crowd []harmony.VoiceState
	for i := range harmony.MaxVoices {
		crowd = append(crowd, voiceState(string(rune('a'+i)), (i*7)%12, 3+i/6))
	}

	alone := renderSummed(t, nil, notes, frames)
	drones := renderSummed(t, crowd, nil, frames)
	together := renderSummed(t, crowd, notes, frames)

	for i := range alone {
		if got := together[i] - drones[i]; math.Abs(got-alone[i]) > 1e-4 {
			t.Fatalf("frame %d: the chime contributes %.6f against twelve drones, but %.6f on its own: the theme's phrase gain does not mean one level",
				i, got, alone[i])
		}
	}
}

func droneCrowd(n int) []harmony.VoiceState {
	var voices []harmony.VoiceState
	for i := range n {
		vs := voiceState(string(rune('a'+i)), (i*7)%12, 3+i/6)
		vs.Expression = harmony.Expression{Intensity: 1, Tremolo: 1, Width: 1}
		voices = append(voices, vs)
	}
	return voices
}

func sustainedRMS(t *testing.T, voices []harmony.VoiceState) float64 {
	t.Helper()
	f := DefaultFormat()
	r, m := NewCaptureRenderer(f, testOpts())
	if err := r.Update(harmony.State{Voices: voices}); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4096*frameSize)
	for range 24 {
		m.Read(buf)
	}

	const windows = 48
	var sq float64
	for range windows {
		m.Read(buf)
		for _, fr := range decodeFrames(buf) {
			sq += fr[0] * fr[0]
		}
	}
	return math.Sqrt(sq / float64(windows*4096))
}

func TestTheMixHoldsItsLevelAcrossVoiceCounts(t *testing.T) {
	solo := sustainedRMS(t, droneCrowd(1))

	for _, n := range []int{2, 4, harmony.MaxVoices} {
		spread := 20 * math.Log10(sustainedRMS(t, droneCrowd(n))/solo)
		if math.Abs(spread) > 3 {
			t.Errorf("%d voices sit %.2f dB from one voice, outside 3 dB: the soundscape changes level as sessions come and go", n, spread)
		}
	}
}

func soloDroneRMS(t *testing.T, silentPeers int) float64 {
	t.Helper()
	f := DefaultFormat()
	r, m := NewCaptureRenderer(f, testOpts())
	if err := r.Update(harmony.State{Voices: droneCrowd(1)}); err != nil {
		t.Fatal(err)
	}
	for i := range silentPeers {
		m.Add(fmt.Sprintf("silent/%d", i), DroneBus, &constSource{})
	}

	buf := make([]byte, 4096*frameSize)
	for range 24 {
		m.Read(buf)
	}

	const windows = 48
	var sq float64
	for range windows {
		m.Read(buf)
		for _, fr := range decodeFrames(buf) {
			sq += fr[0] * fr[0]
		}
	}
	return math.Sqrt(sq / float64(windows*4096))
}

func TestEachDroneStaysAudibleInACrowd(t *testing.T) {
	alone := soloDroneRMS(t, 0)
	inCrowd := soloDroneRMS(t, harmony.MaxVoices-1)

	perVoice := 20 * math.Log10(inCrowd/alone)
	if perVoice < -12 {
		t.Errorf("one drone measures %.2f dB against the same drone sounding alone once %d peers share its bus: below about -12 dB a session stops being identifiable, which is the point of the tool",
			perVoice, harmony.MaxVoices-1)
	}
}

func TestTheCoherentWorstCaseStaysInsideFullScale(t *testing.T) {
	f := DefaultFormat()
	opts := testOpts()
	opts.Volume = 1.0

	var unison []harmony.VoiceState
	for i := range harmony.MaxVoices {
		vs := voiceState(string(rune('a'+i)), 0, 4)
		vs.Expression = harmony.Expression{Intensity: 1, Tremolo: 1, Width: 1}
		unison = append(unison, vs)
	}

	r, m := NewCaptureRenderer(f, opts)
	if err := r.Update(harmony.State{Voices: unison}); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4096*frameSize)
	var peak float64
	for range 72 {
		m.Read(buf)
		for _, fr := range decodeFrames(buf) {
			if a := math.Abs(fr[0]); a > peak {
				peak = a
			}
		}
	}

	if peak > 1.0 {
		t.Errorf("twelve drones in unison peak at %.4f: the coherent worst case must not leave full scale", peak)
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
