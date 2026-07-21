package config

import (
	"reflect"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func luaTable(t *testing.T, source string) (*lua.LState, *lua.LTable) {
	t.Helper()
	L := lua.NewState()
	if err := L.DoString("value = " + source); err != nil {
		L.Close()
		t.Fatal(err)
	}
	table, ok := L.GetGlobal("value").(*lua.LTable)
	if !ok {
		L.Close()
		t.Fatal("source did not produce a table")
	}
	return L, table
}

func TestDecodeLuaConfigStrictOverlay(t *testing.T) {
	L, table := luaTable(t, `{
		layout = {
			channels_width = 31,
			elements = { accounts = { visible = false, width = 12 } },
		},
		media = { max_response_bytes = 123456789 },
		plugins = {
			disabled = {"one", "two"},
			grants = { ["one"] = {"fs", "net"} },
		},
	}`)
	defer L.Close()
	cfg := Config{}
	if err := DecodeLua(&cfg, table); err != nil {
		t.Fatalf("DecodeLua: %v", err)
	}
	if cfg.Layout.ChannelsWidth != 31 || cfg.Layout.GuildsWidth != Default().Layout.GuildsWidth {
		t.Fatalf("layout overlay = %+v", cfg.Layout)
	}
	policy := cfg.Layout.Element("accounts")
	if policy.Visible == nil || *policy.Visible || policy.Width != 12 {
		t.Fatalf("layout.elements.accounts = %+v", policy)
	}
	if cfg.Media.MaxResponseBytes != 123456789 {
		t.Fatalf("signed integer = %d", cfg.Media.MaxResponseBytes)
	}
	if cfg.Plugins == nil || !cfg.Plugins.Enabled || len(cfg.Plugins.Disabled) != 2 || len(cfg.Plugins.Grants["one"]) != 2 {
		t.Fatalf("plugins pointer/slice/map = %+v", cfg.Plugins)
	}
}

func TestDecodeLuaConfigRejectsUnknownWrongAndMachineState(t *testing.T) {
	tests := []struct {
		name, source, path string
	}{
		{"unknown", `{ layout = { mystery = true } }`, "config.layout.mystery"},
		{"wrong type", `{ layout = { channels_width = "wide" } }`, "config.layout.channels_width"},
		{"fractional integer", `{ layout = { channels_width = 2.5 } }`, "config.layout.channels_width"},
		{"accounts unavailable", `{ accounts = {} }`, "config.accounts"},
		{"bad map value", `{ plugins = { grants = { demo = {true} } } }`, "config.plugins.grants.demo[1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			L, table := luaTable(t, tt.source)
			defer L.Close()
			cfg := Default()
			err := DecodeLua(&cfg, table)
			if err == nil || !strings.Contains(err.Error(), tt.path) {
				t.Fatalf("DecodeLua error = %v, want path %q", err, tt.path)
			}
		})
	}
}

func TestDecodeLuaValueSupportsUnsignedAndPointers(t *testing.T) {
	type values struct {
		Unsigned uint64 `toml:"unsigned"`
		Signed   int64  `toml:"signed"`
		Flag     *bool  `toml:"flag"`
	}
	L, table := luaTable(t, `{ unsigned = 42, signed = -9, flag = true }`)
	defer L.Close()
	var got values
	if err := decodeLuaValue(reflectValue(&got), table, "values", luaDecodeOptions{}); err != nil {
		t.Fatal(err)
	}
	if got.Unsigned != 42 || got.Signed != -9 || got.Flag == nil || !*got.Flag {
		t.Fatalf("decoded = %+v", got)
	}
}

func reflectValue(pointer any) reflect.Value {
	return reflect.ValueOf(pointer).Elem()
}
