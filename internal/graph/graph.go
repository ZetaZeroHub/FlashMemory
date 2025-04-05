package graph

import (
	"encoding/json"
	"os"

	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
)

// KnowledgeGraph holds the nodes (functions) and relationships.
type KnowledgeGraph struct {
	Functions []analyzer.LLMAnalysisResult // all function analysis results
	Calls     map[string][]string          // adjacency list: func -> funcs it calls
	CalledBy  map[string][]string          // reverse adjacency: func -> funcs that call it
	Packages  map[string][]string          // package -> functions in that package
	Externals map[string][]string          // external lib -> functions using it
}

// BuildGraph constructs the knowledge graph from analysis results.
func BuildGraph(results []analyzer.LLMAnalysisResult) KnowledgeGraph {
	kg := KnowledgeGraph{
		Functions: results,
		Calls:     make(map[string][]string),
		CalledBy:  make(map[string][]string),
		Packages:  make(map[string][]string),
		Externals: make(map[string][]string),
	}
	for _, res := range results {
		name := res.Func.Name
		// if function has receiver or package, include that in identifier to avoid conflicts
		if res.Func.Package != "" {
			name = res.Func.Package + "." + name
		}
		// Add to package index
		pkg := res.Func.Package
		if pkg == "" {
			pkg = "(root)"
		}
		kg.Packages[pkg] = append(kg.Packages[pkg], name)
		// Add calls relationships
		for _, callee := range res.InternalDeps {
			kg.Calls[name] = append(kg.Calls[name], callee)
			kg.CalledBy[callee] = append(kg.CalledBy[callee], name)
		}
		// Add external usage relationships
		for _, lib := range res.ExternalDeps {
			kg.Externals[lib] = append(kg.Externals[lib], name)
		}
	}
	return kg
}

// SaveGraphJSON saves the graph structure to a JSON file (for debugging or analysis).
func (kg *KnowledgeGraph) SaveGraphJSON(path string) error {
	data, err := json.MarshalIndent(kg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
