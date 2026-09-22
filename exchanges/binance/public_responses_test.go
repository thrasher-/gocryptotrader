package binance

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

// Successful unmarshalling alone misses silently discarded fields. Walk every
// observed object member, including nested objects and every item in an array.
func assertResponseFields(t *testing.T, raw json.RawMessage, typ reflect.Type, path string) {
	t.Helper()
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == reflect.TypeFor[json.RawMessage]() {
		// Opaque, jurisdiction-specific JSON is retained verbatim by the model.
		return
	}
	if len(raw) == 0 {
		return
	}
	switch raw[0] {
	case '[':
		if typ.Kind() != reflect.Slice && typ.Kind() != reflect.Array {
			return
		}
		var rows []json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &rows), "observed array must decode")
		for _, row := range rows {
			assertResponseFields(t, row, typ.Elem(), path+"[]")
		}
	case '{':
		if typ.Kind() == reflect.Slice {
			assertResponseFields(t, raw, typ.Elem(), path)
			return
		}
		if typ.Kind() == reflect.Interface || typ.Kind() == reflect.Map {
			return
		}
		require.Equal(t, reflect.Struct, typ.Kind(), "observed object must have an object model")
		fields := responseJSONFields(typ)
		var object map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &object), "observed object must decode")
		for name, value := range object {
			field, ok := fields[name]
			if !ok {
				for tag, candidate := range fields {
					if strings.EqualFold(tag, name) {
						field, ok = candidate, true
						break
					}
				}
			}
			if assert.Truef(t, ok, "%s.%s should be represented by %s", path, name, typ) {
				assertResponseFields(t, value, field, path+"."+name)
			}
		}
	}
}

func responseJSONFields(typ reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for field := range typ.Fields() {
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if field.Anonymous && name == "" {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				maps.Copy(fields, responseJSONFields(embedded))
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}
