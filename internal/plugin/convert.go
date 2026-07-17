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
