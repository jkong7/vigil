package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jkong7/vigil/internal/check"
	"github.com/jkong7/vigil/internal/state"
)

func TestFromIncidentKind(t *testing.T) {
	start := time.Unix(100, 0)
	i := state.Incident{Monitor: "api", Started: start, Cause: "timeout"}
	if e := FromIncident(i); e.Kind != IncidentOpened || !e.At.Equal(start) {
		t.Fatalf("open incident event = %+v", e)
	}
	end := time.Unix(200, 0)
	i.Resolved = &end
	if e := FromIncident(i); e.Kind != IncidentResolved || !e.At.Equal(end) {
		t.Fatalf("resolved incident event = %+v", e)
	}
}

func TestEventJSONShape(t *testing.T) {
	e := FromResult(check.Result{Monitor: "api", Up: true, Latency: time.Millisecond, At: time.Unix(0, 0).UTC()})
	body, _ := json.Marshal(e)
	var m map[string]any
	json.Unmarshal(body, &m)
	if m["kind"] != "check.completed" || m["monitor"] != "api" || m["incident"] != nil {
		t.Fatalf("unexpected event json: %s", body)
	}
}
