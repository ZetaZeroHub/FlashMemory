package tool

import (
	"context"
	"testing"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	err := reg.Register(ToolSpec{Name: "fm.health", Title: "Health", Version: "v1"}, func(ctx context.Context, req InvokeRequest) (interface{}, error) {
		return map[string]interface{}{"ok": true}, nil
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	dupErr := reg.Register(ToolSpec{Name: "fm.health", Title: "Health", Version: "v1"}, nil)
	if dupErr == nil {
		t.Fatalf("expected duplicate register error")
	}

	items := reg.List()
	if len(items) != 1 || items[0].Name != "fm.health" {
		t.Fatalf("unexpected list result: %+v", items)
	}

	invoke := reg.Invoke(context.Background(), InvokeRequest{Name: "fm.health"})
	if !invoke.Success {
		t.Fatalf("invoke should succeed: %+v", invoke)
	}

	miss := reg.Invoke(context.Background(), InvokeRequest{Name: "fm.unknown"})
	if miss.Success || miss.Error == "" {
		t.Fatalf("expected missing tool error: %+v", miss)
	}
}
