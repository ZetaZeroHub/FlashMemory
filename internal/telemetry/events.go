package telemetry

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	EventSchemaVersion   = "v1"
	EventTypeSearchQuery = "search_query"
	EventTypeToolInvoke  = "tool_invoke"
)

var ErrProjectDirRequired = errors.New("project_dir is required")

type Event struct {
	SchemaVersion string      `json:"schema_version,omitempty"`
	Timestamp     string      `json:"timestamp"`
	EventType     string      `json:"event_type"`
	RequestID     string      `json:"request_id,omitempty"`
	TraceID       string      `json:"trace_id,omitempty"`
	ProjectDir    string      `json:"project_dir,omitempty"`
	Status        string      `json:"status,omitempty"`
	Tool          string      `json:"tool,omitempty"`
	Message       string      `json:"message,omitempty"`
	Cost          *EventCost  `json:"cost,omitempty"`
	Data          interface{} `json:"data,omitempty"`
}

type EventCost struct {
	Hint            string  `json:"hint,omitempty"`
	EstimatedTokens int     `json:"estimated_tokens,omitempty"`
	EstimatedUSD    float64 `json:"estimated_usd,omitempty"`
}

type EventMeta struct {
	SchemaVersion string
	RequestID     string
	TraceID       string
	Cost          *EventCost
}

type SearchEventData struct {
	Query       string      `json:"query,omitempty"`
	SearchMode  string      `json:"search_mode,omitempty"`
	Route       interface{} `json:"route,omitempty"`
	LLMRoute    interface{} `json:"llm_route,omitempty"`
	ResultCount int         `json:"result_count,omitempty"`
}

type ToolEventData struct {
	Tool       string      `json:"tool,omitempty"`
	DurationMs int64       `json:"duration_ms,omitempty"`
	Success    bool        `json:"success"`
	Error      string      `json:"error,omitempty"`
	Input      interface{} `json:"input,omitempty"`
	Output     interface{} `json:"output,omitempty"`
}

var writeMu sync.Mutex

func normalizeEventMeta(meta EventMeta) EventMeta {
	if strings.TrimSpace(meta.SchemaVersion) == "" {
		meta.SchemaVersion = EventSchemaVersion
	}
	meta.RequestID = strings.TrimSpace(meta.RequestID)
	meta.TraceID = strings.TrimSpace(meta.TraceID)
	if meta.TraceID == "" {
		meta.TraceID = meta.RequestID
	}
	return meta
}

func AppendProjectEvent(projectDir string, event Event) error {
	if strings.TrimSpace(projectDir) == "" {
		return ErrProjectDirRequired
	}
	if strings.TrimSpace(event.SchemaVersion) == "" {
		event.SchemaVersion = EventSchemaVersion
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	if event.ProjectDir == "" {
		event.ProjectDir = projectDir
	}
	gitgoDir := filepath.Join(projectDir, ".gitgo")
	if err := os.MkdirAll(gitgoDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(gitgoDir, "events.jsonl")
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}

	writeMu.Lock()
	defer writeMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func RecordSearchEvent(projectDir, requestID, status, message string, data SearchEventData) error {
	return RecordSearchEventWithMeta(projectDir, EventMeta{
		RequestID: requestID,
	}, status, message, data)
}

func RecordSearchEventWithMeta(projectDir string, meta EventMeta, status, message string, data SearchEventData) error {
	meta = normalizeEventMeta(meta)
	return AppendProjectEvent(projectDir, Event{
		SchemaVersion: meta.SchemaVersion,
		EventType:     EventTypeSearchQuery,
		RequestID:     meta.RequestID,
		TraceID:       meta.TraceID,
		Status:        status,
		Message:       message,
		Cost:          meta.Cost,
		Data:          data,
	})
}

func RecordToolEvent(projectDir, requestID, status, toolName, message string, data ToolEventData) error {
	return RecordToolEventWithMeta(projectDir, EventMeta{
		RequestID: requestID,
	}, status, toolName, message, data)
}

func RecordToolEventWithMeta(projectDir string, meta EventMeta, status, toolName, message string, data ToolEventData) error {
	meta = normalizeEventMeta(meta)
	return AppendProjectEvent(projectDir, Event{
		SchemaVersion: meta.SchemaVersion,
		EventType:     EventTypeToolInvoke,
		RequestID:     meta.RequestID,
		TraceID:       meta.TraceID,
		Status:        status,
		Tool:          toolName,
		Message:       message,
		Cost:          meta.Cost,
		Data:          data,
	})
}

func ReadProjectEvents(projectDir string, limit int, eventType, requestID string) ([]Event, error) {
	return ReadProjectEventsWithQuery(projectDir, EventQuery{
		Limit:     limit,
		EventType: eventType,
		RequestID: requestID,
	})
}

type EventQuery struct {
	Limit     int
	EventType string
	RequestID string
	TraceID   string
}

func ReadProjectEventsWithQuery(projectDir string, query EventQuery) ([]Event, error) {
	if strings.TrimSpace(projectDir) == "" {
		return nil, ErrProjectDirRequired
	}
	path := filepath.Join(projectDir, ".gitgo", "events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer f.Close()

	out := make([]Event, 0, 64)
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var ev Event
			if unmarshalErr := json.Unmarshal(line, &ev); unmarshalErr == nil {
				if query.EventType != "" && !strings.EqualFold(ev.EventType, query.EventType) {
					goto next
				}
				if query.RequestID != "" && ev.RequestID != query.RequestID {
					goto next
				}
				if query.TraceID != "" && ev.TraceID != query.TraceID {
					goto next
				}
				out = append(out, ev)
			}
		}
	next:
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	if query.Limit > 0 && len(out) > query.Limit {
		return out[len(out)-query.Limit:], nil
	}
	return out, nil
}
