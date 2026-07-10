package picker_test

import (
	"fmt"

	"awesomeProject/internal/picker"
)

func ExampleFilterEmoji() {
	for _, e := range picker.FilterEmoji("fire", 1) {
		fmt.Printf("%s :%s:\n", e.Char, e.Name)
	}
	// Output:
	// 🔥 :fire:
}

func ExampleEmojiInsert_fakeNitro() {
	// An animated emoji from another guild without Nitro falls back to the CDN
	// URL, which tuicord renders back as the emoji.
	text, ok := picker.EmojiInsert(1084, "blobDance", true, false, false, true)
	fmt.Println(ok)
	fmt.Println(text)
	// Output:
	// true
	// https://cdn.discordapp.com/emojis/1084.gif?size=48&name=blobDance
}

func ExampleEmojiInsert_native() {
	// A static emoji from a guild the user is in inserts as a native mention.
	text, _ := picker.EmojiInsert(77, "thonk", false, true, false, true)
	fmt.Println(text)
	// Output: <:thonk:77>
}
