package uistate_test

import (
	"fmt"

	"awesomeProject/internal/uistate"
)

// Toggling a pin returns the new status, which the caller persists.
func ExampleState_TogglePinnedChannel() {
	st := &uistate.State{}
	fmt.Println(st.TogglePinnedChannel(12345)) // pin
	fmt.Println(st.IsPinnedChannel(12345))
	fmt.Println(st.TogglePinnedChannel(12345)) // unpin
	// Output:
	// true
	// true
	// false
}
