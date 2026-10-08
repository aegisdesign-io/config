// The config package provides a simple application configuration system with
// strongly-typed objects. Configuration is read from either a YAML or JSON
// file and returned to the application.
package config

import (
	"encoding"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v4"
)

// FromFile reads application configuration from a file found at `path`, then
// returns the decoded data to the application-supplied object, konf, which
// must be a non-nil pointer to a struct.
//
// Errors reading the file, decoding the file, or if the file is of an
// unexpected type are returned.
func FromFile(path string, konf interface{}) error {
	var unmarshal func([]byte, interface{}) error
	switch t := strings.ToLower(filepath.Ext(path)); t {
	case ".json":
		unmarshal = json.Unmarshal
	case ".yaml", ".yml":
		unmarshal = yaml.Unmarshal
	default:
		return fmt.Errorf("config: unexpected file format: %s", t)
	}

	if err := checkTarget(konf); err != nil {
		return err
	}

	buf, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return decode(buf, konf, unmarshal)
}

// Decodes buf into the configuration object, then applies environment
// overrides. The caller must have validated konf with checkTarget.
func decode(buf []byte, konf interface{}, unmarshal func([]byte, interface{}) error) error {
	if err := unmarshal(buf, konf); err != nil {
		return fmt.Errorf("config: while decoding: %w", err)
	}

	return fromEnvironment(konf)
}

// Ensures the configuration object is a non-nil pointer to a struct.
func checkTarget(konf interface{}) error {
	v := reflect.ValueOf(konf)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("config: target must be a non-nil pointer to a struct, got %T", konf)
	}
	return nil
}

func fromEnvironment(konf interface{}) error {
	w := envWalker{onPath: map[reflect.Type]bool{}, seen: map[seenKey]bool{}}
	_, err := w.walk(reflect.ValueOf(konf).Elem(), "")
	return err
}

// Returns the struct type a field leads to, either directly or through a
// pointer, or nil if it is neither.
func nestedStruct(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct {
		return t
	}
	return nil
}

// Caches, per struct type, whether an `env` tag is reachable from it.
var envTagCache sync.Map

// Reports whether t, or any struct type reachable from it, has a field with an
// `env` tag. Unexported fields are included so that a variable aimed inside
// one is reported as an error rather than silently ignored.
func hasEnvTag(t reflect.Type) bool {
	if v, ok := envTagCache.Load(t); ok {
		return v.(bool)
	}
	visited := map[reflect.Type]bool{}
	var search func(reflect.Type) bool
	search = func(t reflect.Type) bool {
		if v, ok := envTagCache.Load(t); ok {
			return v.(bool)
		}
		if visited[t] {
			return false
		}
		visited[t] = true
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			_, tagged := f.Tag.Lookup("env")
			if st := nestedStruct(f.Type); tagged || (st != nil && search(st)) {
				// A positive result holds regardless of where the search
				// started, so it is safe to cache for nested types too. A
				// negative one may be cut short by a cycle, so only the
				// top-level result is cached.
				envTagCache.Store(t, true)
				return true
			}
		}
		return false
	}
	found := search(t)
	envTagCache.Store(t, found)
	return found
}

// Walks a configuration tree setting fields from the environment. onPath holds
// the struct types on the current path, and a nil pointer to one of them is
// never allocated, so a self-referential type is not expanded indefinitely;
// seen holds the structs already reached through pointers, so pointer cycles
// terminate.
type envWalker struct {
	onPath map[reflect.Type]bool
	seen   map[seenKey]bool
}

// Identifies a struct reached through a pointer. The type is part of the key
// because a struct and its first field share an address.
type seenKey struct {
	addr uintptr
	typ  reflect.Type
}

// Depth-first search of a struct for fields with an `env` tag. Reports whether
// any field was set from the environment. path is the dotted name of the
// struct, used in error messages.
//
// Unexported fields are walked like any other. Their values are read-only, so
// an error is returned only where the environment would actually set
// something inside one.
func (w envWalker) walk(valueOfKonf reflect.Value, path string) (bool, error) {
	typeOfKonf := valueOfKonf.Type()
	found := false

	if !w.onPath[typeOfKonf] {
		w.onPath[typeOfKonf] = true
		defer delete(w.onPath, typeOfKonf)
	}

	for i := 0; i < valueOfKonf.NumField(); i++ {
		valueOfField := valueOfKonf.Field(i)
		typeOfField := typeOfKonf.Field(i)
		name := path + typeOfField.Name

		if st := nestedStruct(typeOfField.Type); st != nil && hasEnvTag(st) {
			set, err := w.walkNested(valueOfField, name)
			if err != nil {
				return false, err
			}
			found = found || set
		}

		// If the field contains an `env` tag, attempt to load the value from the
		// environment. An empty value overrides plain string fields and is
		// treated as unset for all other types.
		tag, ok := typeOfField.Tag.Lookup("env")
		if !ok {
			continue
		}
		val, ok := os.LookupEnv(tag)
		if !ok || (val == "" && !emptyOverrides(typeOfField.Type)) {
			continue
		}
		if !valueOfField.CanSet() {
			return false, fmt.Errorf("config: env %s for field %s: field is not settable (unexported?)", tag, name)
		}
		if err := setFromString(valueOfField, val); err != nil {
			return false, fmt.Errorf("config: env %s for field %s: %w", tag, name, err)
		}
		found = true
	}

	return found, nil
}

// Walks a struct or pointer-to-struct field. A nil pointer is only allocated
// if the environment sets something inside it.
func (w envWalker) walkNested(valueOfField reflect.Value, name string) (bool, error) {
	if valueOfField.Kind() == reflect.Struct {
		return w.walk(valueOfField, name+".")
	}
	if !valueOfField.IsNil() {
		key := seenKey{valueOfField.Pointer(), valueOfField.Type().Elem()}
		if w.seen[key] {
			return false, nil
		}
		w.seen[key] = true
		return w.walk(valueOfField.Elem(), name+".")
	}

	elem := valueOfField.Type().Elem()
	if w.onPath[elem] {
		return false, nil
	}

	x := reflect.New(elem)
	set, err := w.walk(x.Elem(), name+".")
	if err != nil || !set {
		return false, err
	}
	if !valueOfField.CanSet() {
		return false, fmt.Errorf("config: field %s: field is not settable (unexported?)", name)
	}
	valueOfField.Set(x)
	return true, nil
}

// Reports whether an empty env value overrides a field of type t: true for
// strings and pointers to them, unless the type decodes itself via
// encoding.TextUnmarshaler.
func emptyOverrides(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.String && !reflect.PtrTo(t).Implements(textUnmarshalerType)
}

var textUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// Converts val to match the field and sets it. Types implementing
// encoding.TextUnmarshaler decode themselves; everything else is converted
// by kind.
func setFromString(field reflect.Value, val string) error {
	if field.Kind() != reflect.Ptr && field.CanAddr() {
		if u, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return u.UnmarshalText([]byte(val))
		}
	}

	switch field.Kind() {
	case reflect.Ptr:
		// Write through an existing pointer so that anything sharing it sees
		// the override.
		if !field.IsNil() {
			return setFromString(field.Elem(), val)
		}
		x := reflect.New(field.Type().Elem())
		if err := setFromString(x.Elem(), val); err != nil {
			return err
		}
		field.Set(x)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x, err := strconv.ParseInt(val, 10, field.Type().Bits())
		if err != nil {
			// time.Duration also accepts duration strings like "30s". Other
			// named types can implement encoding.TextUnmarshaler.
			if field.Type() != durationType {
				return err
			}
			d, derr := time.ParseDuration(val)
			if derr != nil {
				return fmt.Errorf("%w (and not a duration: %v)", err, derr)
			}
			x = int64(d)
		}
		field.SetInt(x)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		x, err := strconv.ParseUint(val, 10, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetUint(x)
	case reflect.Bool:
		x, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		field.SetBool(x)
	case reflect.Float32, reflect.Float64:
		x, err := strconv.ParseFloat(val, field.Type().Bits())
		if err != nil {
			return err
		}
		field.SetFloat(x)
	case reflect.String:
		field.SetString(val)
	default:
		return fmt.Errorf("unsupported field type %s", field.Type())
	}
	return nil
}

var durationType = reflect.TypeOf(time.Duration(0))
