#!/usr/bin/env python3
"""Draws the picture behind the icons in the installer window.

Run it when the design changes and commit the result: the build only uses
background.tiff, so it needs no Python packages. It needs Pillow.

    python3 macos/dmg/make-background.py
"""
import subprocess
from pathlib import Path
from PIL import Image, ImageDraw, ImageFilter, ImageFont

here = Path(__file__).parent
W, H = 660, 400                 # the window, in points
APP, APPS = (180, 215), (480, 215)  # where the two icons sit; see build.sh

CANVAS, TEXT, MUTED, ACCENT = (15, 15, 19), (236, 236, 241), (169, 169, 182), (163, 168, 251)


def font(size, weight="regular"):
    # SF is on every Mac. Variable font: pick a named weight.
    f = ImageFont.truetype("/System/Library/Fonts/SFNS.ttf", size)
    try:
        f.set_variation_by_name({"regular": "Regular", "semibold": "Semibold", "bold": "Bold"}[weight])
    except Exception:
        pass
    return f


def draw(scale):
    s = scale
    img = Image.new("RGB", (W * s, H * s), CANVAS)

    # A soft glow of the brand colour behind each icon, so the two read as targets.
    glow = Image.new("RGB", img.size, (0, 0, 0))
    g = ImageDraw.Draw(glow)
    for cx, cy in (APP, APPS):
        r = 120 * s
        g.ellipse((cx * s - r, cy * s - r, cx * s + r, cy * s + r), fill=(48, 50, 110))
    glow = glow.filter(ImageFilter.GaussianBlur(70 * s))
    img = Image.blend(img, Image.composite(glow, img, glow.convert("L").point(lambda v: min(255, v * 3))), 0.9)

    d = ImageDraw.Draw(img)

    # The ant, from the app's own artwork.
    ant = Image.open(here.parent / "Sources/Taasant/Resources/ant.png").convert("RGBA")
    aw = 64 * s
    ant = ant.resize((aw, int(ant.height * aw / ant.width)), Image.NEAREST)
    img.paste(ant, ((W * s - aw) // 2, 28 * s), ant)

    def centred(text, y, f, fill):
        w = d.textlength(text, font=f)
        d.text(((W * s - w) / 2, y * s), text, font=f, fill=fill)

    centred("Install taasant", 88, font(26 * s, "semibold"), TEXT)
    centred("Drag the app onto Applications", 124, font(14 * s), MUTED)

    # An arrow from the app to Applications, dashed, with a head.
    y = APP[1] * s
    x0, x1 = (APP[0] + 78) * s, (APPS[0] - 78) * s
    x = x0
    while x < x1 - 16 * s:
        d.line((x, y, x + 9 * s, y), fill=ACCENT, width=3 * s)
        x += 17 * s
    d.polygon([(x1, y), (x1 - 15 * s, y - 9 * s), (x1 - 15 * s, y + 9 * s)], fill=ACCENT)

    centred("Then open taasant from Applications. It lives in your menu bar.", 338, font(13 * s), MUTED)
    centred("The first time, macOS may ask you to confirm: System Settings → Privacy & Security → Open Anyway.", 358, font(11 * s), (110, 110, 124))
    return img


draw(1).save(here / "background.png")
draw(2).save(here / "background@2x.png")
# One file that holds both sizes, so the window is sharp on a Retina screen.
subprocess.run(["tiffutil", "-cathidpicheck", here / "background.png", here / "background@2x.png",
                "-out", here / "background.tiff"], check=True)
(here / "background.png").unlink()
(here / "background@2x.png").unlink()
print("wrote", here / "background.tiff")
