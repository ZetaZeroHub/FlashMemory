package dart

//#include "src/parser.c"
//#include "src/scanner.c"
import "C"

import (
	"unsafe"
	sitter "github.com/smacker/go-tree-sitter"
)

func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_dart())
	return sitter.NewLanguage(ptr)
}
