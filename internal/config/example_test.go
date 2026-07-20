package config_test

import (
	"fmt"

	"awesomeProject/internal/config"
)

// Default provides a complete configuration used when no file is present.
func ExampleDefault() {
	cfg := config.Default()
	fmt.Println("channels width:", cfg.Layout.ChannelsWidth)
	fmt.Println("quick switcher:", cfg.Keys.QuickSwitcher)
	// Output:
	// channels width: 20
	// quick switcher: ctrl+k
}
