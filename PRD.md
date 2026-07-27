# PRD: Hum
## Ambient Auditory Display for Autonomous Work

**Version:** 0.1 (MVP)

---

# 1. Vision

Hum is a local-first daemon that converts computational work into an evolving musical soundscape.

Instead of watching terminals, dashboards or notifications, developers perceive system state through ambient sound.

Hum is **not** a notification system.

Hum is an **auditory display engine** that renders work session lifecycle events as music.

The long-term vision is to make computational state something a developer can hear rather than continuously monitor visually.

---

# 2. Goals

## Primary Goals

- Represent active work through ambient sound.
- Eliminate unnecessary context switching.
- Support multiple simultaneous work sessions.
- Be independent of any AI provider or agent framework.
- Be lightweight enough to run continuously.
- Feel like a Unix daemon.

## Non-Goals

Hum is not:

- an AI agent framework
- an LLM orchestration tool
- a task manager
- a project planner
- a notification manager
- an IDE plugin

Hum only renders work.

---

# 3. Design Philosophy

Hum should feel like:

- Git
- tmux
- OpenTelemetry

Small.

Composable.

Generic.

Framework agnostic.

Hum should know nothing about:

- Claude
- Codex
- Gemini
- Oh My Pi
- Cursor
- GitHub

These become integrations.

Hum only understands work sessions.

---

# 4. Architecture

```
               Clients

 Claude Code
 Codex
 Oh My Pi
 Shell
 GitHub Actions
 CI
 Custom Scripts

        │
        ▼

     Hum Event API

        │
        ▼

      Hum Daemon

 ┌──────────────────────┐
 │ Session Registry     │
 │ Configuration        │
 │ Harmony Engine       │
 │ Renderer             │
 │ Audio Engine         │
 └──────────────────────┘

        │
        ▼

 Speakers
```

Clients emit generic events.

Hum owns all rendering.

---

# 5. Core Concept

## One Work Session = One Voice

A work session represents one piece of work.

Examples:

- Terraform validation
- Pull request review
- Documentation generation
- Research
- Build
- Deployment

Internally that work may create:

- 1 agent
- 5 agents
- 100 agents

Hum does not care.

One work session always maps to one sustained musical voice.

---

# 6. Session Lifecycle

```
session.started

↓

Allocate musical voice

↓

Fade in drone

↓

Remain active

↓

session.completed

↓

Completion phrase

↓

Fade out drone
```

Failure uses a different completion phrase.

---

# 7. Musical Model

User selects:

- Key
- Scale
- Theme

Example:

```
Key:
D

Scale:
Minor Pentatonic

Theme:
Orchestra
```

Hum allocates notes from the chosen scale.

Example:

```
Session 1
D2

Session 2
F2

Session 3
A2

Session 4
C3
```

More sessions create richer harmony.

Completed sessions disappear.

---

# 8. Session Completion

When a session completes:

- Play the same pitch class one or two octaves higher.
- Fade out the drone.

Example:

```
Working
D2

Completion
D5

Drone removed
```

---

# 9. Failure

Failure should never be an alarm.

Instead use a recognisable musical cadence.

Examples:

- descending interval
- unresolved suspension
- soft dissonance

The goal is recognition.

Not punishment.

---

# 10. Internal Agent Behaviour

Sub-agents do NOT receive additional notes.

Instead they affect expression.

Examples:

- richer harmonics
- stereo widening
- tremolo
- resonance
- modulation

The session's pitch remains constant.

---

# 11. Themes

Themes define how Hum sounds.

Examples:

## Orchestra

- cello
- viola
- clarinet
- french horn

## Monastery

- Gregorian drones
- chant completion phrases

## Synthwave

- analogue pads
- bass drones

## Minimal

- sine waves only

## Nature

- water
- wind
- birds

Themes should be easily extensible.

---

# 12. Configuration

Global configuration

```
~/.hum/config.yaml
```

Project configuration

```
project/.hum/config.yaml
```

Resolution order

```
CLI

↓

Project

↓

Global

↓

Defaults
```

---

# 13. Example Project Config

```yaml
project:
  name: tofu

music:
  root: D
  scale: dorian
  theme: orchestra
```

---

# 14. Event Protocol

The protocol must remain generic.

Hum knows nothing about AI.

Core events:

```
session.started

session.updated

session.completed

session.failed

session.cancelled
```

Example:

```json
{
  "event": "session.started",
  "id": "123",
  "workspace": "tofu",
  "title": "Validate PR #142"
}
```

Completion:

```json
{
  "event": "session.completed",
  "id": "123"
}
```

No provider-specific fields belong in the protocol.

---

# 15. Session Object

```yaml
id:
workspace:
title:
state:
priority:
metadata:
```

---

# 16. Transport

MVP:

Unix Domain Socket

Future:

- HTTP
- WebSocket
- Named pipes

Transport should be abstracted behind an interface.

---

# 17. CLI

```
hum init

hum start

hum stop

hum complete

hum fail

hum status

hum mute

hum doctor

hum theme list

hum theme use orchestra
```

Daemon:

```
humd
```

---

# 18. Repository Structure

```
hum/

cmd/
    hum/
    humd/

internal/

    config/

    session/

    protocol/

    harmony/

    renderer/

    audio/

themes/

samples/

docs/
```

---

# 19. Renderer Architecture

Core should never directly produce sound.

```
Session Events

↓

Harmony Engine

↓

Renderer

↓

Output
```

Possible renderers:

- Audio
- MIDI
- OSC
- Philips Hue
- Stream Deck
- Apple Watch
- Haptics

The renderer system should be pluggable.

---

# 20. MVP Scope

Implement:

- daemon
- CLI
- Unix socket API
- session registry
- note allocation
- project/global config
- one built-in theme
- sine-wave renderer
- completion phrase
- failure phrase
- volume control
- mute
- graceful shutdown

Do NOT implement:

- networking
- cloud sync
- GUI
- multiple renderer types
- plugins
- MIDI
- physical modelling

---

# 21. Future Roadmap

Phase 2

- sampled instruments
- MIDI renderer
- multiple themes

Phase 3

- renderer plugins
- OSC
- Home Assistant
- Philips Hue
- Stream Deck

Phase 4

- collaborative orchestras
- shared sessions
- distributed event bus

---

# 22. Engineering Principles

- Local-first.
- Generic event protocol.
- Renderer abstraction.
- Zero AI-provider knowledge.
- Configuration over code.
- Minimal dependencies.
- Cross-platform.
- Deterministic behaviour.
- Unix philosophy.
- Stable public protocol.

---

# 23. Success Criteria

A developer should be able to:

- Start Hum.
- Launch work from any compatible tool.
- Hear new work begin.
- Hear multiple concurrent work sessions form harmonious chords.
- Hear work complete without looking at a screen.
- Understand workload through ambient sound alone.

If Hum causes users to stop repeatedly checking terminal windows for agent progress, the MVP has achieved its objective.
