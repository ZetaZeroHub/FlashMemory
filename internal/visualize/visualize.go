package visualize

import (
	"fmt"
	"strings"

	"github.com/kinglegendzzh/flashmemory/internal/graph"
)

// Stats holds some basic metrics for a file or package.
type Stats struct {
	Item          string // file or package name
	FunctionCount int
	TotalLines    int
	ImportCount   int
	FanIn         int // how many calls into this (if package-level, how many other packages call into this package)
	FanOut        int // how many calls out
}

// ComputePackageStats aggregates stats by package using the knowledge graph.
func ComputePackageStats(kg graph.KnowledgeGraph) []Stats {
	stats := []Stats{}
	// We will aggregate by package
	for pkg, funcs := range kg.Packages {
		st := Stats{Item: pkg}
		funcSet := make(map[string]bool)
		for _, f := range funcs {
			funcSet[f] = true
			st.FunctionCount++
			// find function analysis to get lines and imports
			for _, res := range kg.Functions {
				fname := res.Func.Name
				if res.Func.Package != "" {
					fname = res.Func.Package + "." + fname
				}
				if fname == f {
					st.TotalLines += res.Func.Lines
					st.ImportCount += len(res.ExternalDeps)
					// fan-out: any external call from functions in this pkg count
					// fan-in and fan-out at package level might be calculated later using call graph
				}
			}
		}
		stats = append(stats, st)
	}
	// Compute inter-package fan-in/fan-out
	for caller, callees := range kg.Calls {
		// determine packages of caller and callee
		pkgCaller := strings.Split(caller, ".")[0]
		for _, callee := range callees {
			pkgCallee := strings.Split(callee, ".")[0]
			if pkgCaller != pkgCallee {
				// external package call
				// increment fan-out of caller pkg and fan-in of callee pkg
				for i := range stats {
					if stats[i].Item == pkgCaller {
						stats[i].FanOut++
					}
					if stats[i].Item == pkgCallee {
						stats[i].FanIn++
					}
				}
			}
		}
	}
	return stats
}

// PrintStats outputs a summary of stats per package.
func PrintStats(stats []Stats) {
	fmt.Println("Package Statistics:")
	for _, st := range stats {
		fmt.Printf("- Package %s: %d functions, %d total lines, imports: %d, fan-in: %d, fan-out: %d\n",
			st.Item, st.FunctionCount, st.TotalLines, st.ImportCount, st.FanIn, st.FanOut)
	}
}
