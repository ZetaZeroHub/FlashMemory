package parser

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// createTempFile 辅助函数，用于创建临时文件并写入指定内容
func createTempFile(t *testing.T, suffix, content string) string {
	tmpFile, err := ioutil.TempFile("", "test*."+suffix)
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write([]byte(content)); err != nil {
		t.Fatalf("写入临时文件失败: %v", err)
	}
	return tmpFile.Name()
}

// TestParseFile_Go_Complex 针对 Go 语言的复杂样本测试
func TestParseFile_Go_Complex(t *testing.T) {
	code := `package test

// 简单函数
func Hello() {
    println("Hello, World!")
}

// 带参数函数
func Add(a int, b int) int {
    return a + b
}

// 定义结构体
type MyType struct {}

// 方法：带接收器的函数，函数体为空
func (m *MyType) Method() {
    // 空方法体
}
`
	fileName := createTempFile(t, "go", code)
	defer os.Remove(fileName)

	parser := &TreeSitterParser{Lang: "go"}

	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Go 代码解析失败: %v", err)
	}
	// 预期解析到3个函数：Hello, Add, Method
	if len(funcs) != 3 {
		t.Errorf("预期解析到3个函数，实际解析到 %d 个函数", len(funcs))
	}
	expected := map[string]bool{"Hello": true, "Add": true, "Method": true}
	for _, f := range funcs {
		if _, ok := expected[f.Name]; !ok {
			t.Errorf("解析到未知函数: %s", f.Name)
		}
	}
}

func TestParseFile_Go_File(t *testing.T) {
	parser := &TreeSitterParser{Lang: "go"}
	// Get the absolute path to the test file
	testFilePath, err := filepath.Abs("testdata/test_samples.go")
	if err != nil {
		t.Fatalf("Failed to get absolute path: %v", err)
	}

	// Parse the test file
	funcs, err := parser.ParseFile(testFilePath)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// Verify that we found the expected functions
	if len(funcs) == 0 {
		t.Errorf("No functions found in test file")
	}

	// Create a map for easier lookup
	funcMap := make(map[string]FunctionInfo)
	for _, fn := range funcs {
		funcMap[fn.Name] = fn
		t.Log("Function:", fn.Name, "Receiver:", fn.Receiver, "Calls:", fn.Calls)
	}
}

// TestParseFile_Java_Complex 针对 Java 语言的复杂样本测试
func TestParseFile_Java_Complex(t *testing.T) {
	code := `package test;

public class TestClass {
    // 构造函数（不应被捕获）
    public TestClass() {}

    // 普通方法
    public void hello() {
        System.out.println("Hello, World!");
    }

    // 静态方法，复杂格式
    public static int add(int a, int b) {
        return a + b;
    }

    // 带额外空格和注释的方法
    public String greet( String name ) {
        // 向用户问候
        return "Hello, " + name;
    }
}
`
	fileName := createTempFile(t, "java", code)
	defer os.Remove(fileName)

	parser := &TreeSitterParser{Lang: "java"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Java 代码解析失败: %v", err)
	}
	// 预期只捕获普通方法：hello, add, greet（构造函数不匹配）
	if len(funcs) != 3 {
		t.Errorf("预期解析到3个方法，实际解析到 %d 个方法", len(funcs))
	}
	expected := map[string]bool{"hello": true, "add": true, "greet": true}
	for _, f := range funcs {
		if _, ok := expected[f.Name]; !ok {
			t.Errorf("解析到未知方法: %s", f.Name)
		}
	}
}

// TestParseFile_Python_Complex 针对 Python 语言的复杂样本测试
func TestParseFile_Python_Complex(t *testing.T) {
	code := `# 顶级函数
def hello():
    # 嵌套函数
    def nested():
        pass
    print("Hello, World!")

def add(a, b):
    return a + b

# 带额外空格和注释的函数
def greet(name):
    # 问候函数
    return "Hello, " + name
`
	fileName := createTempFile(t, "py", code)
	defer os.Remove(fileName)

	parser := &TreeSitterParser{Lang: "python"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Python 代码解析失败: %v", err)
	}
	// 预期捕获到4个函数：hello, nested, add, greet
	if len(funcs) != 4 {
		t.Errorf("预期解析到4个函数，实际解析到 %d 个函数", len(funcs))
	}
	expected := map[string]bool{"hello": true, "nested": true, "add": true, "greet": true}
	for _, f := range funcs {
		if _, ok := expected[f.Name]; !ok {
			t.Errorf("解析到未知函数: %s", f.Name)
		}
	}
}

// TestParseFile_Cpp_Complex 针对 C++ 语言的复杂样本测试
func TestParseFile_Cpp_Complex(t *testing.T) {
	code := `#include <iostream>

// 全局函数
void hello() {
    std::cout << "Hello, World!" << std::endl;
}

// 带返回值的函数
int add(int a, int b) {
    return a + b;
}

// 含额外空格和注释的函数
double compute ( double x ) {
    // 计算平方
    return x * x;
}
`
	fileName := createTempFile(t, "cpp", code)
	defer os.Remove(fileName)

	parser := &TreeSitterParser{Lang: "cpp"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("C++ 代码解析失败: %v", err)
	}
	// 预期捕获到3个函数：hello, add, compute
	if len(funcs) != 3 {
		t.Errorf("预期解析到3个函数，实际解析到 %d 个函数", len(funcs))
	}
	expected := map[string]bool{"hello": true, "add": true, "compute": true}
	for _, f := range funcs {
		if _, ok := expected[f.Name]; !ok {
			t.Errorf("解析到未知函数: %s", f.Name)
		}
	}
}

// TestParseFile_JavaScript_Complex 针对 JavaScript 语言的复杂样本测试
func TestParseFile_JavaScript_Complex(t *testing.T) {
	code := `// 函数声明
function hello() {
    console.log("Hello, World!");
}

// 带额外格式的函数
function add(a, b) {
    return a + b;
}

// 包含嵌套函数的示例
function outer() {
    function inner() {
        return 42;
    }
    return inner();
}
`
	fileName := createTempFile(t, "js", code)
	defer os.Remove(fileName)

	parser := &TreeSitterParser{Lang: "javascript"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("JavaScript 代码解析失败: %v", err)
	}
	// 预期捕获到4个函数：hello, add, outer, inner
	if len(funcs) != 4 {
		t.Errorf("预期解析到4个函数，实际解析到 %d 个函数", len(funcs))
	}
	expected := map[string]bool{"hello": true, "add": true, "outer": true, "inner": true}
	for _, f := range funcs {
		if _, ok := expected[f.Name]; !ok {
			t.Errorf("解析到未知函数: %s", f.Name)
		}
	}
}

// TestParseFile_ErrorCases 测试错误情况，例如不支持的语言和文件不存在
func TestParseFile_ErrorCases(t *testing.T) {
	// 测试不支持的语言
	parser := &TreeSitterParser{Lang: "unsupported"}
	_, err := parser.ParseFile("nonexistent.file")
	if err == nil {
		t.Error("预期不支持的语言返回错误，但未返回错误")
	}

	// 测试文件不存在
	parser = &TreeSitterParser{Lang: "go"}
	_, err = parser.ParseFile("nonexistent.go")
	if err == nil {
		t.Error("预期不存在的文件返回错误，但未返回错误")
	}
}
