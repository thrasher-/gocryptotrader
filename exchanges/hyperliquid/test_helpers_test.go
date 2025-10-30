package hyperliquid

import (
	"io"
	"net/http"
	"testing"

	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

func mustCloseBody(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Fatalf("body must close: %v", err)
	}
}

func mustDecodeJSON(t *testing.T, reader io.Reader, target any) {
	t.Helper()
	if err := json.NewDecoder(reader).Decode(target); err != nil {
		t.Fatalf("decode json must not error: %v", err)
	}
}

func mustEncodeJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode json must not error: %v", err)
	}
}

func requireFloatAny(t *testing.T, value any, name string) float64 {
	t.Helper()
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if v == "" {
			t.Fatalf("%s must parse as float: empty string", name)
		}
		var num types.Number
		if err := num.UnmarshalJSON([]byte(`"` + v + `"`)); err != nil {
			t.Fatalf("%s must parse as float: %v", name, err)
		}
		return float64(num)
	default:
		t.Fatalf("%s must be numeric, got %T", name, value)
	}
	return 0
}

func mustReadAll(t *testing.T, reader io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read must not error: %v", err)
	}
	return data
}

func mustUnmarshalJSON(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal json must not error: %v", err)
	}
}
