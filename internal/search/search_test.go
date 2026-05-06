package search

import (
	"testing"

	"github.com/kinglegendzzh/flashmemory/internal/index"
)

func TestQueryPassesKeywordModelToCompletion(t *testing.T) {
	tmp := t.TempDir()
	db, err := index.EnsureIndexDB(tmp)
	if err != nil {
		t.Fatalf("EnsureIndexDB failed: %v", err)
	}
	defer db.Close()

	oldCompletion := keywordCompletion
	defer func() { keywordCompletion = oldCompletion }()

	var gotModel string
	keywordCompletion = func(query, model string) (string, error) {
		gotModel = model
		return `["auth"]`, nil
	}

	se := &SearchEngine{
		Indexer: &index.Indexer{DB: db},
		ProjDir: tmp,
	}
	_, err = se.Query("auth", SearchOptions{
		SearchMode:   "keyword",
		KeywordModel: "routed-model",
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if gotModel != "routed-model" {
		t.Fatalf("expected routed keyword model, got %q", gotModel)
	}
}
