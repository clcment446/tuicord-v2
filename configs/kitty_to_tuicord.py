#!/usr/bin/env python3
"""Convert a Kitty color-scheme file into a tuicord Lua theme."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


def parse_kitty_theme(contents: str) -> dict[str, str]:
    """Parse Kitty's simple ``name value`` color directives."""
    colors: dict[str, str] = {}
    for raw_line in contents.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(None, 1)
        if len(parts) == 2:
            name, value = parts
            colors[name] = value.strip()
    return colors


def _slugify(name: str) -> str:
    slug = re.sub(r"[^a-zA-Z0-9_-]+", "-", name.strip()).strip("-_").lower()
    return slug or "kitty-theme"


def _color(colors: dict[str, str], *names: str) -> str:
    for name in names:
        if colors.get(name):
            return colors[name]
    raise ValueError(f"missing Kitty color: {names[0]}")


def _optional_color(colors: dict[str, str], names: tuple[str, ...], fallback: str) -> str:
    return next((colors[name] for name in names if colors.get(name)), fallback)


def convert(contents: str, theme_name: str) -> str:
    """Return a complete tuicord theme declaration for Kitty ``contents``."""
    colors = parse_kitty_theme(contents)
    background = _color(colors, "background")
    text = _color(colors, "foreground")
    accent = _optional_color(
        colors, ("color5", "color4", "active_border_color"), text
    )
    border = _optional_color(
        colors, ("active_border_color", "color4", "color5"), accent
    )

    palette = {
        "background": background,
        "text": text,
        "muted": _color(colors, "color8", "foreground"),
        "accent": accent,
        "selection": _color(colors, "selection_background", "color8", "foreground"),
        "border": border,
        "error": _optional_color(colors, ("color3", "color1", "color9"), accent),
    }
    slug = _slugify(theme_name)

    lines = [
        f'-- Converted from Kitty theme: {theme_name}',
        f'tuicord.theme("{slug}", {{',
        "  palette = {",
    ]
    lines.extend(f'    {key} = "{value}",' for key, value in palette.items())
    lines.extend(
        [
            "  },",
            "})",
            f'tuicord.use_theme("{slug}")',
            "",
        ]
    )
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Convert a Kitty .conf theme into a tuicord config.lua theme"
    )
    parser.add_argument("theme", help="Kitty theme path, or - to read stdin")
    parser.add_argument(
        "-n", "--name", help="tuicord theme name (defaults to the input filename)"
    )
    parser.add_argument("-o", "--output", type=Path, help="write Lua to this file")
    args = parser.parse_args(argv)

    contents = sys.stdin.read() if args.theme == "-" else Path(args.theme).read_text()
    name = args.name or ("kitty-theme" if args.theme == "-" else Path(args.theme).stem)
    result = convert(contents, name)
    if args.output:
        args.output.write_text(result)
    else:
        sys.stdout.write(result)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
