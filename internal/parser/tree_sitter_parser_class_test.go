package parser

import (
	"os"
	"testing"
)

// TestExtractClassName_Python 测试Python语言类名提取功能
func TestExtractClassName_Python(t *testing.T) {
	code := `# Python test with class
class MyClass:
    def __init__(self):
        self.value = 42
        
    def method1(self):
        print("Hello from method1")
        
    def method2(self, param):
        return self.value + param

# Another class
class AnotherClass:
    pass
`
	fileName := createTempFile(t, "py", code)
	defer os.Remove(fileName)

	parser := &TreeSitterParser{Lang: "python", Debug: true}

	// Parse the file to extract functions and class information
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Python代码解析失败: %v", err)
	}

	// Check if we have functions from the class
	if len(funcs) < 3 {
		t.Errorf("预期至少解析到3个方法，实际解析到 %d 个方法", len(funcs))
	}

	// Check if any function has the class name in its package field
	foundClass := false
	for _, f := range funcs {
		if f.Package == "MyClass" || f.Package == "AnotherClass" {
			foundClass = true
			t.Logf("Found method with class: %s in class %s", f.Name, f.Package)
		}
	}

	if !foundClass {
		t.Errorf("未能提取到Python类名")
	}
}
