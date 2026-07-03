package widget_test

import (
	"fmt"

	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/widget"
)

// ItemList draws each row's label on the left and its badge right-aligned.
func ExampleItemList_Draw() {
	list := widget.NewItemList([]widget.Item{
		{Label: "general"},
		{Label: "random", Badge: "3"},
	})

	buf := screen.NewBuffer(10, 2)
	list.Draw(buf.Clip(buf.Bounds()))

	fmt.Printf("%q\n", row(buf, 0))
	fmt.Printf("%q\n", row(buf, 1))
	// Output:
	// "general   "
	// "random   3"
}
