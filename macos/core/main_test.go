package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const link = "mierus://alice:pw@example.com?port=2012&profile=alice&protocol=TCP"

func call(t *testing.T, method string, req any, daemon bool) (any, error) {
	t.Helper()
	b, _ := json.Marshal(req)
	return dispatch(method, b, daemon)
}

func TestStatelessCalls(t *testing.T) {
	res, err := call(t, "parse", map[string]string{"text": link}, false)
	if err != nil {
		t.Fatal(err)
	}
	var ps []map[string]any
	if err := json.Unmarshal(res.(json.RawMessage), &ps); err != nil || len(ps) != 1 || ps[0]["server"] != "example.com" {
		t.Fatalf("parse: %v %s", err, res)
	}
	profile, _ := json.Marshal(ps[0])

	l, err := call(t, "link", map[string]any{"profile": json.RawMessage(profile)}, false)
	if err != nil || !strings.HasPrefix(l.(string), "mierus://alice:") {
		t.Fatal(l, err)
	}

	rt, _ := call(t, "defaultRouting", nil, false)
	opts := map[string]any{
		"profile": json.RawMessage(profile), "routing": rt, "stack": "gvisor",
		"desktop": map[string]any{"perAppMode": "bypass", "apps": []string{"/Applications/Safari.app"}},
	}
	cfg, err := call(t, "preview", map[string]any{"options": opts}, false)
	if err != nil || !strings.Contains(cfg.(string), `"auto-route": true`) || !strings.Contains(cfg.(string), "PROCESS-PATH-REGEX") {
		t.Fatalf("preview: %v\n%v", err, cfg)
	}

	if _, err := call(t, "start", map[string]any{"options": opts}, false); err == nil {
		t.Fatal("start must need the daemon")
	}
	if _, err := call(t, "nope", nil, false); err == nil {
		t.Fatal("unknown method accepted")
	}
	if _, err := dispatch("parse", []byte("{"), false); err == nil {
		t.Fatal("bad json accepted")
	}
}

func TestEnvelope(t *testing.T) {
	b, _ := json.Marshal(envelope(json.RawMessage(`{"a":1}`), nil))
	if string(b) != `{"ok":true,"result":{"a":1}}` {
		t.Fatal(string(b))
	}
	b, _ = json.Marshal(envelope(nil, errString("x")))
	if string(b) != `{"ok":false,"error":"x"}` {
		t.Fatal(string(b))
	}
}

type errString string

func (e errString) Error() string { return string(e) }
