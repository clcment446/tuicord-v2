package config

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// DecodeLua overlays a strict Lua table onto Default and stores the result in
// dst. Table keys are the existing toml tags, so the Lua and legacy TOML
// formats share one typed runtime schema. Accounts and internal-only fields are
// deliberately unavailable: accounts are machine-managed UI state, not authored
// configuration.
func DecodeLua(dst *Config, table *lua.LTable) error {
	if dst == nil {
		return fmt.Errorf("config: nil destination")
	}
	if table == nil {
		return fmt.Errorf("config: want table, got nil")
	}

	next := Default()
	// A present plugins table changes the nil-as-default representation into an
	// explicit value. Preserve the documented conceptual default for fields the
	// table does not mention.
	if table.RawGetString("plugins") != lua.LNil {
		next.Plugins = &Plugins{Enabled: true}
	}
	if err := decodeLuaValue(reflect.ValueOf(&next).Elem(), table, "config", luaDecodeOptions{rootConfig: true}); err != nil {
		return err
	}
	if err := ValidateColors(next.Colors); err != nil {
		return fmt.Errorf("config.colors: %w", err)
	}
	*dst = next
	return nil
}

type luaDecodeOptions struct {
	rootConfig bool
}

func decodeLuaValue(dst reflect.Value, value lua.LValue, path string, opts luaDecodeOptions) error {
	if !dst.CanSet() {
		return fmt.Errorf("%s: destination is not settable", path)
	}
	if dst.Kind() == reflect.Pointer {
		if value == lua.LNil {
			dst.SetZero()
			return nil
		}
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return decodeLuaValue(dst.Elem(), value, path, luaDecodeOptions{})
	}

	switch dst.Kind() {
	case reflect.Struct:
		table, ok := value.(*lua.LTable)
		if !ok {
			return luaTypeError(path, "table", value)
		}
		fields := make(map[string]int, dst.NumField())
		for i := 0; i < dst.NumField(); i++ {
			field := dst.Type().Field(i)
			name := strings.Split(field.Tag.Get("toml"), ",")[0]
			if name == "" {
				name = strings.ToLower(field.Name)
			}
			if name == "-" || field.PkgPath != "" {
				continue
			}
			if opts.rootConfig && name == "accounts" {
				continue
			}
			fields[name] = i
		}
		var decodeErr error
		table.ForEach(func(key, child lua.LValue) {
			if decodeErr != nil {
				return
			}
			name, ok := key.(lua.LString)
			if !ok {
				decodeErr = fmt.Errorf("%s: object key must be a string, got %s", path, key.Type().String())
				return
			}
			index, ok := fields[string(name)]
			if !ok {
				decodeErr = fmt.Errorf("%s.%s: unknown field", path, name)
				return
			}
			decodeErr = decodeLuaValue(dst.Field(index), child, path+"."+string(name), luaDecodeOptions{})
		})
		return decodeErr

	case reflect.Bool:
		v, ok := value.(lua.LBool)
		if !ok {
			return luaTypeError(path, "boolean", value)
		}
		dst.SetBool(bool(v))
		return nil

	case reflect.String:
		v, ok := value.(lua.LString)
		if !ok {
			return luaTypeError(path, "string", value)
		}
		dst.SetString(string(v))
		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := luaInteger(path, value)
		if err != nil {
			return err
		}
		if dst.OverflowInt(n) {
			return fmt.Errorf("%s: integer %v is out of range for %s", path, value, dst.Type())
		}
		dst.SetInt(n)
		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := luaUnsigned(path, value)
		if err != nil {
			return err
		}
		if dst.OverflowUint(n) {
			return fmt.Errorf("%s: integer %v is out of range for %s", path, value, dst.Type())
		}
		dst.SetUint(n)
		return nil

	case reflect.Slice:
		table, ok := value.(*lua.LTable)
		if !ok {
			return luaTypeError(path, "array", value)
		}
		length := table.Len()
		var keyErr error
		table.ForEach(func(key, _ lua.LValue) {
			if keyErr != nil {
				return
			}
			n, ok := key.(lua.LNumber)
			if !ok || float64(n) != math.Trunc(float64(n)) || n < 1 || int(n) > length {
				keyErr = fmt.Errorf("%s: array keys must be contiguous integers 1..%d", path, length)
			}
		})
		if keyErr != nil {
			return keyErr
		}
		out := reflect.MakeSlice(dst.Type(), length, length)
		for i := 1; i <= length; i++ {
			if err := decodeLuaValue(out.Index(i-1), table.RawGetInt(i), fmt.Sprintf("%s[%d]", path, i), luaDecodeOptions{}); err != nil {
				return err
			}
		}
		dst.Set(out)
		return nil

	case reflect.Map:
		table, ok := value.(*lua.LTable)
		if !ok {
			return luaTypeError(path, "table", value)
		}
		if dst.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("%s: unsupported map key type %s", path, dst.Type().Key())
		}
		out := reflect.MakeMap(dst.Type())
		var decodeErr error
		table.ForEach(func(key, child lua.LValue) {
			if decodeErr != nil {
				return
			}
			name, ok := key.(lua.LString)
			if !ok {
				decodeErr = fmt.Errorf("%s: map key must be a string, got %s", path, key.Type().String())
				return
			}
			entry := reflect.New(dst.Type().Elem()).Elem()
			entryPath := path + "." + string(name)
			if err := decodeLuaValue(entry, child, entryPath, luaDecodeOptions{}); err != nil {
				decodeErr = err
				return
			}
			out.SetMapIndex(reflect.ValueOf(string(name)).Convert(dst.Type().Key()), entry)
		})
		if decodeErr != nil {
			return decodeErr
		}
		dst.Set(out)
		return nil
	default:
		return fmt.Errorf("%s: unsupported destination type %s", path, dst.Type())
	}
}

func luaInteger(path string, value lua.LValue) (int64, error) {
	n, ok := value.(lua.LNumber)
	if !ok {
		return 0, luaTypeError(path, "integer", value)
	}
	f := float64(n)
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < -math.Exp2(63) || f >= math.Exp2(63) {
		return 0, fmt.Errorf("%s: want signed integer, got %v", path, value)
	}
	return int64(f), nil
}

func luaUnsigned(path string, value lua.LValue) (uint64, error) {
	n, ok := value.(lua.LNumber)
	if !ok {
		return 0, luaTypeError(path, "unsigned integer", value)
	}
	f := float64(n)
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < 0 || f >= math.Exp2(64) {
		return 0, fmt.Errorf("%s: want unsigned integer, got %v", path, value)
	}
	return uint64(f), nil
}

func luaTypeError(path, want string, value lua.LValue) error {
	got := "nil"
	if value != nil {
		got = value.Type().String()
	}
	return fmt.Errorf("%s: want %s, got %s", path, want, got)
}
