package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"awesomeProject/internal/tui/input"
	"awesomeProject/internal/tui/term"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return term.Run(func(t *term.Terminal) error {
		reader := input.NewReader(ctx, t)
		_, _ = t.Write([]byte("\x1b[2J\x1b[Hinput inspector: press keys, paste, click, or move the mouse. q/Ctrl+C exits.\r\n\r\n"))
		for {
			select {
			case ev, ok := <-reader.Events():
				if !ok {
					return nil
				}
				if shouldQuit(ev) {
					return nil
				}
				_, _ = t.Write([]byte(fmt.Sprintf("%s\r\n", describe(ev))))
			case err, ok := <-reader.Errors():
				if ok && err != nil {
					return err
				}
			case <-time.After(5 * time.Minute):
				return nil
			}
		}
	})
}

func shouldQuit(ev input.Event) bool {
	key, ok := ev.(input.KeyEvent)
	return ok && !key.Release && key.Key == input.KeyRune &&
		(key.Rune == 'q' || key.Rune == 'c' && key.Mods&input.Ctrl != 0)
}

func describe(ev input.Event) string {
	switch ev := ev.(type) {
	case input.KeyEvent:
		return fmt.Sprintf("key key=%v rune=%q mods=%v release=%v", ev.Key, ev.Rune, ev.Mods, ev.Release)
	case input.MouseEvent:
		return fmt.Sprintf("mouse kind=%v button=%v x=%d y=%d mods=%v", ev.Kind, ev.Btn, ev.X, ev.Y, ev.Mods)
	case input.PasteEvent:
		return fmt.Sprintf("paste %q", ev.Text)
	case input.FocusEvent:
		return fmt.Sprintf("focus focused=%v", ev.Focused)
	default:
		return fmt.Sprintf("%T", ev)
	}
}
