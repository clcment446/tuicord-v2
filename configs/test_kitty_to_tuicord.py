import unittest

from kitty_to_tuicord import convert, parse_kitty_theme


AYAKA = """
# Theme: Ayaka
foreground #cedaeb
background #36283d
selection_foreground #000000
selection_background #cedaeb
active_border_color #71ADE9
color1 #71ADE9
color3 #E59DB1
color5 #8BB8E9
color8 #9098a4
"""


class KittyToTuicordTest(unittest.TestCase):
    def test_parses_comments_and_directives(self):
        colors = parse_kitty_theme(AYAKA)

        self.assertEqual(colors["foreground"], "#cedaeb")
        self.assertEqual(colors["color5"], "#8BB8E9")
        self.assertNotIn("Theme:", colors)

    def test_converts_kitty_colors_to_complete_lua_theme(self):
        output = convert(AYAKA, "Ayaka")

        self.assertIn('tuicord.theme("ayaka", {', output)
        self.assertIn('background = "#36283d"', output)
        self.assertIn('text = "#cedaeb"', output)
        self.assertIn('muted = "#9098a4"', output)
        self.assertIn('accent = "#8BB8E9"', output)
        self.assertIn('selection = "#cedaeb"', output)
        self.assertIn('border = "#71ADE9"', output)
        self.assertIn('error = "#E59DB1"', output)
        self.assertIn('tuicord.use_theme("ayaka")', output)

    def test_missing_required_colors_are_reported(self):
        with self.assertRaisesRegex(ValueError, "missing Kitty color: background"):
            convert("foreground #ffffff\n", "incomplete")

    def test_theme_name_is_safe_for_lua(self):
        output = convert("foreground #fff\nbackground #000\n", "My Theme!")

        self.assertIn('tuicord.theme("my-theme", {', output)


if __name__ == "__main__":
    unittest.main()
