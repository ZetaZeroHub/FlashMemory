package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendProjectEvent(t *testing.T) {
	proj := t.TempDir()
	err := AppendProjectEvent(proj, Event{
		EventType: "tool_invoke",
		RequestID: "req-1",
		Status:    "success",
		Tool:      "fm.health.check",
	})
	if err != nil {
		t.Fatalf("append event failed: %v", err)
	}
	path := filepath.Join(proj, ".gitgo", "events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read event file failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "\"event_type\":\"tool_invoke\"") {
		t.Fatalf("event_type not found: %s", text)
	}
	if !strings.Contains(text, "\"tool\":\"fm.health.check\"") {
		t.Fatalf("tool not found: %s", text)
	}
	if !strings.Contains(text, "\"schema_version\":\"v1\"") {
		t.Fatalf("schema_version not found: %s", text)
	}
}

func TestReadProjectEvents(t *testing.T) {
	proj := t.TempDir()
	if err := RecordSearchEvent(proj, "r1", "success", "search completed", SearchEventData{
		Query:       "login",
		SearchMode:  "semantic",
		ResultCount: 2,
	}); err != nil {
		t.Fatalf("record search event failed: %v", err)
	}
	if err := RecordToolEvent(proj, "r2", "success", "fm.intent.route", "OK", ToolEventData{
		Tool:    "fm.intent.route",
		Success: true,
	}); err != nil {
		t.Fatalf("record tool event failed: %v", err)
	}

	all, err := ReadProjectEvents(proj, 10, "", "")
	if err != nil {
		t.Fatalf("read all events failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}

	toolOnly, err := ReadProjectEvents(proj, 10, EventTypeToolInvoke, "")
	if err != nil {
		t.Fatalf("read tool events failed: %v", err)
	}
	if len(toolOnly) != 1 || toolOnly[0].EventType != EventTypeToolInvoke {
		t.Fatalf("unexpected tool filter result: %+v", toolOnly)
	}

	byReq, err := ReadProjectEvents(proj, 10, "", "r1")
	if err != nil {
		t.Fatalf("read by request_id failed: %v", err)
	}
	if len(byReq) != 1 || byReq[0].RequestID != "r1" {
		t.Fatalf("unexpected request_id filter result: %+v", byReq)
	}

	if err := RecordSearchEventWithMeta(proj, EventMeta{
		RequestID: "r3",
		TraceID:   "trace-1",
		Cost: &EventCost{
			Hint:            "medium",
			EstimatedTokens: 2048,
		},
	}, "success", "search completed", SearchEventData{
		Query: "trace query",
	}); err != nil {
		t.Fatalf("record search event with meta failed: %v", err)
	}

	byTrace, err := ReadProjectEventsWithQuery(proj, EventQuery{
		Limit:   10,
		TraceID: "trace-1",
	})
	if err != nil {
		t.Fatalf("read by trace_id failed: %v", err)
	}
	if len(byTrace) != 1 || byTrace[0].TraceID != "trace-1" {
		t.Fatalf("unexpected trace_id filter result: %+v", byTrace)
	}
	if byTrace[0].Cost == nil || byTrace[0].Cost.Hint != "medium" {
		t.Fatalf("expected cost payload, got: %+v", byTrace[0])
	}
}
