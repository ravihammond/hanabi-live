# PROTOTYPE — Hanabi.live HSM debugging interface

The selected hybrid direction for the Hanabi.live HSM debugging interface:
option B's top toolbar and option A's right inspector, revised after live
prototype feedback.

This is throwaway, read-only UI code. It uses mock diagnostic data and does not
connect to Hanabi.live, HSM replay, game authority, or action selection.

Run from the repository root:

```bash
uv run python -m http.server 4173 --directory third_party/hanabi-live/packages/client/prototypes/hsm-debug-interface
```

Then open:

```text
http://localhost:4173/
```

## Question

What concrete interface should Hanabi.live use for observer-relative HSM
debugging, temporal inspection, classification, faults, and separately gated
physical truth?

## Selected direction

- The real Hanabi.live spectator game and replay controls are the intended
  production host; the prototype mirrors their textured table, cards, replay
  shuttle, image controls, typography, and translucent black controls.
- A retractable inspector lives on the right and can be restored from a
  persistent edge handle.
- Historical replay remains the default. Hindsight adds a clearly marked
  future anchor without granting future knowledge to past actors.
- Card selection provides both structured observer-relative diagnostics and a
  flexible plain-text output stream.
- Turn-action badges preview follow, violation, and neutral/unknown outcomes.

The generated concept references are in [`concepts/`](./concepts/).
