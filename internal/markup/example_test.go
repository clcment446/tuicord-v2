package markup_test

import (
	"fmt"
	"time"

	"awesomeProject/internal/markup"
)

// ExampleParse demonstrates basic entity and markdown parsing.
func ExampleParse() {
	res := markup.Resolver{
		Member:  func(id uint64) (string, bool) { return "alice", true },
		Channel: func(id uint64) (string, bool) { return "general", true },
	}
	for _, span := range markup.Parse("hey <@42>, see <#7> **now**", res) {
		fmt.Printf("%d %q\n", span.Kind, span.Text)
	}
	// Output:
	// 0 "hey "
	// 6 "@alice"
	// 0 ", see "
	// 7 "#general"
	// 0 " "
	// 1 "now"
}

// ExampleParse_roleMention demonstrates role mention parsing with color.
func ExampleParse_roleMention() {
	res := markup.Resolver{
		Role: func(id uint64) (string, uint32, bool) {
			if id == 42 {
				return "Admin", 0xFF0000, true
			}
			return "", 0, false
		},
	}
	spans := markup.Parse("ping <@&42>!", res)
	for _, s := range spans {
		if s.Kind == markup.Kind_RoleMention {
			fmt.Printf("role=%q color=%#x\n", s.Text, s.FG)
		}
	}
	// Output:
	// role="@Admin" color=0xff0000
}

// ExampleParse_timestamp demonstrates timestamp entity parsing.
// Injecting Now into the Resolver keeps relative-time output deterministic.
func ExampleParse_timestamp() {
	res := markup.Resolver{
		Now: func() time.Time {
			return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		},
	}
	// 2022-04-01 12:30:00 UTC
	spans := markup.Parse("<t:1648812600:D>", res)
	fmt.Println(spans[0].Text)
	// Output:
	// 01 April 2022
}

// ExampleParse_discordLink demonstrates recognition of bare Discord channel
// and message URLs embedded in message body text.
func ExampleParse_discordLink() {
	res := markup.Resolver{
		Channel: func(id uint64) (string, bool) {
			if id == 7 {
				return "general", true
			}
			return "", false
		},
	}
	spans := markup.Parse("https://discord.com/channels/1/7/99", res)
	s := spans[0]
	fmt.Printf("kind=%v text=%q target=%q\n", s.Kind, s.Text, s.Action.Target)
	// Output:
	// kind=15 text="#general ↷ 99" target="1/7/99"
}

// ExampleParse_inviteLink demonstrates bare discord.gg invite link recognition.
func ExampleParse_inviteLink() {
	spans := markup.Parse("join: https://discord.gg/abc123", markup.Resolver{})
	for _, s := range spans {
		if s.Kind == markup.Kind_InviteLink {
			fmt.Printf("text=%q code=%q\n", s.Text, s.Action.Target)
		}
	}
	// Output:
	// text="discord.gg/abc123" code="abc123"
}
