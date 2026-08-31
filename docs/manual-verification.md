# Manual verification

`mise run e2e` proves the assembled system behaves as specified: the right
pitches, the documented transposition, a descending failure cadence, a mixer
that stays inside unity, and a soundscape that decays to exact silence. What it
cannot prove is that the result sounds *good*, which is the actual product
requirement. PRD.md §23 is written in terms of what a developer hears, so the
checks below need ears.

Run them on a machine with a working audio device, from a real release build
(`mise run build`, or `brew install hum`), before shipping a release that
changes anything in `internal/audio`, `internal/harmony` or a theme file.

## Setup

```sh
humd &
```

Leave the volume where you would actually keep it — these are judgements about
an ambient display, not about whether a tone is audible when you go looking for
it.

## §23 checks

| # | PRD §23 | What to do | What must be true |
|---|---|---|---|
| 1 | Start Hum | `humd` | Silence. Nothing announces itself. |
| 2 | Launch work from any compatible tool | `hum start --id build --title Build` | A drone **fades in**. No click, no sudden onset. |
| 3 | Hear new work begin | `hum start --id tests --title Tests` | The second voice is distinguishable from the first *without counting*. |
| 4 | Hear concurrent sessions form harmonious chords | run four sessions at once | The four voices sound like one chord, not four unrelated tones. No beating or roughness between neighbours. |
| 5 | Hear work complete without looking at a screen | `hum complete --id build` | The completion phrase is recognisable as "finished" from another room, and is not mistakable for the failure phrase. |
| 6 | — | `hum fail --id tests` | The failure phrase reads as "went wrong" without being alarming. Clearly different from completion. |
| 7 | Understand workload through ambient sound alone | drive a real agent workload for ten minutes with your terminal covered | You can tell how many things are running and when something finished, without looking. |
| 8 | — | leave one session sounding for an hour | The drone does not become irritating. This is the check that decides whether Hum is usable at all. |

## Checks the buffer assertions cannot make

- **Attack and release shapes.** `TestTheSoundscapeDecaysToSilence` proves the
  buffer reaches zero. It cannot say the fade sounded smooth.
- **Shutdown.** `hum daemon stop` with four voices sounding must fade out, not cut.
  The e2e suite asserts the ordering in the log; only listening confirms it.
- **Mute.** `hum mute` then `hum unmute` while a chord is sounding must ramp,
  not step.
- **Volume.** `hum volume 0.2` must not change the timbre, only the level.
- **Norm ramp on voice count change.** When a second session starts while one is
  already sounding, the existing drone must not dip before recovering. When a
  session ends, the remaining drone must not jump. Both are smooth if the ramp
  is working; only listening on a real device confirms it. Covered by issue #101.
- **Register.** `music.octave: 2` must sound deeper without becoming muddy, and
  `music.octave: 5` must not become shrill. Both are taste calls the range
  check in `config.Validate` cannot make.
- **Theme.** Any new theme file needs checks 4, 5 and 6 re-run, because a theme
  changes the phrase spec and the drone envelope.

## Recording the result

Note the version, the platform, the audio device and which checks you ran in
the release PR. A check nobody ran is not a check that passed.
