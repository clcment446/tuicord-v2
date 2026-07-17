package plugin

import (
	"strconv"

	lua "github.com/yuin/gopher-lua"
)

// toLua converts a Go value into a Lua value for handing to plugin callbacks.
//
// uint64 is treated as a Discord snowflake and rendered as a decimal string,
// because Lua numbers are float64 and cannot represent large snowflakes
// exactly. Plain counts should therefore use int/int64, which map to Lua
// numbers. Nested map[string]any and []any become tables.
func toLua(L *lua.LState, v any) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case string:
		return lua.LString(val)
	case uint64:
		return lua.LString(strconv.FormatUint(val, 10))
	case int:
		return lua.LNumber(val)
	case int64:
		return lua.LNumber(val)
	case float64:
		return lua.LNumber(val)
	case map[string]any:
		tbl := L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, toLua(L, item))
		}
		return tbl
	case []any:
		tbl := L.NewTable()
		for _, item := range val {
			tbl.Append(toLua(L, item))
		}
		return tbl
	default:
		return lua.LNil
	}
}

// tableToStringMap reads a Lua table of string keys into a Go map, coercing
// each value to a string (booleans become "true"/"false"). Non-string keys and
// nested tables are skipped. It backs tuicord.style option tables.
func tableToStringMap(tbl *lua.LTable) map[string]string {
	out := make(map[string]string)
	if tbl == nil {
		return out
	}
	tbl.ForEach(func(k, v lua.LValue) {
		key, ok := k.(lua.LString)
		if !ok {
			return
		}
		switch val := v.(type) {
		case lua.LString:
			out[string(key)] = string(val)
		case lua.LBool:
			out[string(key)] = boolString(bool(val))
		case lua.LNumber:
			out[string(key)] = val.String()
		}
	})
	return out
}

// tableToStringSlice reads a Lua array table into a Go string slice, coercing
// each element to a string. It backs tuicord.overlay line lists.
func tableToStringSlice(tbl *lua.LTable) []string {
	if tbl == nil {
		return nil
	}
	n := tbl.Len()
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, tbl.RawGetInt(i).String())
	}
	return out
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// parseID reads a snowflake from a Lua value, accepting either the decimal
// string form plugins normally use or a Lua number. It returns 0 for anything
// unparseable, which the Host functions treat as a no-op.
func parseID(v lua.LValue) uint64 {
	switch val := v.(type) {
	case lua.LString:
		id, err := strconv.ParseUint(string(val), 10, 64)
		if err != nil {
			return 0
		}
		return id
	case lua.LNumber:
		if val < 0 {
			return 0
		}
		return uint64(val)
	default:
		return 0
	}
}
