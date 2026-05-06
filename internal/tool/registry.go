package tool

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrToolExists   = errors.New("tool already exists")
	ErrToolNotFound = errors.New("tool not found")
)

type SchemaField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type ToolSpec struct {
	Name        string        `json:"name"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Version     string        `json:"version"`
	Category    string        `json:"category,omitempty"`
	InputSchema []SchemaField `json:"input_schema,omitempty"`
}

type InvokeRequest struct {
	Name       string                 `json:"tool"`
	ProjectDir string                 `json:"project_dir,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
	Input      map[string]interface{} `json:"input,omitempty"`
}

type InvokeResponse struct {
	Tool       string      `json:"tool"`
	Success    bool        `json:"success"`
	DurationMs int64       `json:"duration_ms"`
	Output     interface{} `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type Handler func(ctx context.Context, req InvokeRequest) (interface{}, error)

type Registry struct {
	mu       sync.RWMutex
	specs    map[string]ToolSpec
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		specs:    make(map[string]ToolSpec),
		handlers: make(map[string]Handler),
	}
}

func (r *Registry) Register(spec ToolSpec, handler Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.specs[spec.Name]; exists {
		return ErrToolExists
	}
	r.specs[spec.Name] = spec
	r.handlers[spec.Name] = handler
	return nil
}

func (r *Registry) RegisterOrReplace(spec ToolSpec, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs[spec.Name] = spec
	r.handlers[spec.Name] = handler
}

func (r *Registry) List() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ToolSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func (r *Registry) Invoke(ctx context.Context, req InvokeRequest) InvokeResponse {
	start := time.Now()
	resp := InvokeResponse{Tool: req.Name}

	r.mu.RLock()
	handler, ok := r.handlers[req.Name]
	r.mu.RUnlock()
	if !ok || handler == nil {
		resp.Success = false
		resp.Error = ErrToolNotFound.Error()
		resp.DurationMs = time.Since(start).Milliseconds()
		return resp
	}

	out, err := handler(ctx, req)
	resp.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		return resp
	}
	resp.Success = true
	resp.Output = out
	return resp
}
