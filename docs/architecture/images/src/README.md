# Architecture diagram sources

The PNGs in the parent directory are generated from this folder. They are
referenced by [`docs/architecture/lifecycle.md`](../../lifecycle.md).

## Regenerating

```bash
make diagrams          # from the repo root
```

or directly:

```bash
./generate.sh
```

Requires **Python 3** and **Node.js** (the renderer is fetched with `npx`; no
install step). No Python dependencies — the SVGs are emitted as plain text.

## Layout

| File | Purpose |
|---|---|
| `theme.py` | The image configuration: NVIDIA dark palette, fonts, and the shared shape primitives (`box`, `arrow`, `curve`, `stub`). Restyle here without touching geometry. |
| `diagrams.py` | One function per diagram, holding its layout and copy. |
| `generate.sh` | Entry point. Emits SVGs to a temp dir, renders them to PNG, cleans up. |

## What is and is not committed

`diagrams.py` is the source of truth; the PNGs are the artifact the docs
reference. The intermediate SVGs are **not** committed — a third representation
of the same drawing is just a way for them to drift.

## Editing notes

**Always open the rendered PNGs before committing.** Overlapping connectors,
labels colliding with dividers, and boxes overflowing their container are
invisible in the source and obvious in the image. Every layout defect in these
diagrams was found by looking, not by reading.

Two SVG behaviours are easy to reintroduce:

- **A dashed path can end on a gap in its dash pattern**, which leaves the
  arrowhead visually detached from its line. Draw the run with
  `curve(..., marker=False)` and put the arrowhead on a short solid `stub()`.
- **Marker `orient` must be `auto`.** `auto-start-reverse` only affects
  `marker-start`, and misorients the vertical arrows here.

Rendering is pinned to a specific `resvg` version in `generate.sh` so an
upstream release cannot silently change how the committed PNGs look. Fonts
resolve from the host, so text metrics can shift slightly between machines;
that only matters if you are diffing a regenerated PNG you did not otherwise
change.
