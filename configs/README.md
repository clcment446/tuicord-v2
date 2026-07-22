# Config helpers

`kitty_to_tuicord.py` converts a Kitty color theme into a complete tuicord Lua
theme declaration. It uses Kitty's semantic colors where available and falls
back to ANSI colors for themes that omit optional directives.

Convert a local Kitty theme directly into a config snippet:

```sh
python3 configs/kitty_to_tuicord.py ~/.config/kitty/current-theme.conf \
  --name ayaka
```

Write the result to a file instead:

```sh
python3 configs/kitty_to_tuicord.py Ayaka.conf -n ayaka \
  --output /tmp/ayaka-theme.lua
```

Copy the generated `tuicord.theme(...)` block into
`~/.config/tuicord-v2/config.lua`. The converter does not modify either Kitty
or tuicord configuration files automatically.

Run its tests with:

```sh
python3 -m unittest discover -s configs -p 'test_*.py'
```
