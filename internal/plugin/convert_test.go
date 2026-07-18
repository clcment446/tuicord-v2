package plugin

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestToLuaSnowflakeIsString(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	// A snowflake beyond float64's exact range must round-trip as a string.
	const snowflake = uint64(9007199254740993) // 2^53 + 1
	if got := toLua(L, snowflake); got.String() != "9007199254740993" {
		t.Fatalf("uint64 -> %q, want decimal string", got.String())
	}
	// Plain counts stay numbers.
	if got := toLua(L, 42); got.Type() != lua.LTNumber {
		t.Fatalf("int -> %s, want number", got.Type())
	}
}

func TestParseID(t *testing.T) {
	cases := []struct {
		in   lua.LValue
		want uint64
	}{
		{lua.LString("9007199254740993"), 9007199254740993},
		{lua.LNumber(111), 111},
		{lua.LString("not-a-number"), 0},
		{lua.LNumber(-5), 0},
		{lua.LNil, 0},
	}
	for _, c := range cases {
		if got := parseID(c.in); got != c.want {
			t.Errorf("parseID(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
