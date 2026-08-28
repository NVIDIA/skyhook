# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""NVIDIA dark theme and SVG primitives for the architecture diagrams.

This module is the image configuration: every colour, font and shape used by
diagrams.py lives here, so a restyle never touches diagram geometry.
"""

BG        = "#0D0D0D"
PANEL     = "#161616"
GREEN     = "#76B900"
GREEN_DIM = "#4F7D00"
GREEN_LIT = "#8FD400"
ON_GREEN  = "#000000"
TEXT      = "#EDEDED"
MUTE      = "#9B9B9B"
STROKE    = "#2E2E2E"
DASH      = "#6E6E6E"

FONT = "'Helvetica Neue', Helvetica, Arial, sans-serif"
MONO = "'SF Mono', Menlo, monospace"


def head(w, h):
    return f'''<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">
<defs>
<marker id="ag" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto">
  <path d="M0,1 L9,5 L0,9 z" fill="{GREEN}"/>
</marker>
<marker id="am" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto">
  <path d="M0,1 L9,5 L0,9 z" fill="{DASH}"/>
</marker>
</defs>
<rect width="{w}" height="{h}" fill="{BG}"/>'''


def title(x, y, s, size=26):
    return (f'<text x="{x}" y="{y}" font-family="{FONT}" font-size="{size}" font-weight="600" '
            f'fill="{TEXT}" letter-spacing="0.2">{s}</text>')


def sub(x, y, s, size=14, fill=None, anchor="start", mono=False):
    f = MONO if mono else FONT
    return (f'<text x="{x}" y="{y}" font-family="{f}" font-size="{size}" '
            f'fill="{fill or MUTE}" text-anchor="{anchor}">{s}</text>')


def box(x, y, w, h, label, kind="solid", note=None, note2=None, r=7, fs=16):
    """kind: solid (work) | outline (check) | neutral | dashed"""
    if kind == "solid":
        fill, stroke, tc, sw, da = GREEN, GREEN, ON_GREEN, 1, ""
    elif kind == "outline":
        fill, stroke, tc, sw, da = PANEL, GREEN, GREEN_LIT, 2, ""
    elif kind == "dashed":
        fill, stroke, tc, sw, da = PANEL, DASH, MUTE, 2, ' stroke-dasharray="6 5"'
    else:
        fill, stroke, tc, sw, da = PANEL, STROKE, TEXT, 1.5, ""
    cx, cy = x + w / 2, y + h / 2
    ty = cy + fs * 0.35 if not note else cy - 2
    out = [f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="{r}" fill="{fill}" '
           f'stroke="{stroke}" stroke-width="{sw}"{da}/>',
           f'<text x="{cx}" y="{ty}" font-family="{MONO if kind in ("solid","outline") else FONT}" '
           f'font-size="{fs}" font-weight="600" fill="{tc}" text-anchor="middle">{label}</text>']
    if note:
        nc = ON_GREEN if kind == "solid" else MUTE
        out.append(f'<text x="{cx}" y="{cy + 16}" font-family="{FONT}" font-size="12" '
                   f'fill="{nc}" text-anchor="middle" opacity="0.85">{note}</text>')
    if note2:
        nc = ON_GREEN if kind == "solid" else MUTE
        out.append(f'<text x="{cx}" y="{cy + 31}" font-family="{FONT}" font-size="12" '
                   f'fill="{nc}" text-anchor="middle" opacity="0.85">{note2}</text>')
    return "\n".join(out)


def arrow(x1, y1, x2, y2, muted=False, label=None, lx=None, ly=None):
    c = DASH if muted else GREEN
    m = "am" if muted else "ag"
    s = (f'<path d="M{x1},{y1} L{x2},{y2}" stroke="{c}" stroke-width="2" fill="none" '
         f'marker-end="url(#{m})"/>')
    if label:
        s += sub(lx or (x1 + x2) / 2, ly or (y1 + y2) / 2 - 8, label, 12, MUTE, "middle")
    return s


def curve(d, muted=False, label=None, lx=0, ly=0, dash=False, marker=True):
    c = DASH if muted else GREEN
    m = "am" if muted else "ag"
    da = ' stroke-dasharray="6 5"' if dash else ""
    mk = f' marker-end="url(#{m})"' if marker else ""
    s = f'<path d="{d}" stroke="{c}" stroke-width="2" fill="none"{mk}{da}/>' 
    if label:
        s += sub(lx, ly, label, 12, MUTE, "middle")
    return s


def stub(x1, y1, x2, y2, muted=True):
    """Short SOLID segment carrying the arrowhead. A dashed path can terminate on a
    gap in its dash pattern, which leaves the marker visually detached from the line."""
    c = DASH if muted else GREEN
    m = "am" if muted else "ag"
    return (f'<path d="M{x1},{y1} L{x2},{y2}" stroke="{c}" stroke-width="2" '
            f'fill="none" marker-end="url(#{m})"/>')
