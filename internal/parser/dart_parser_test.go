package parser

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestDartParser(t *testing.T) {
	content := `import 'package:flutter/material.dart';

class MyApp extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return MaterialApp();
  }
}

void main() {
  runApp(MyApp());
}
`
	tmpFile, err := ioutil.TempFile("", "test_*.dart")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	parser := TreeSitterParser{Lang: "dart", Debug: true} // Enable debug
	funcs, err := parser.ParseFile(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	for i, f := range funcs {
		fmt.Printf("Func %d: Name=%s, Type=%s, Pkg=%s, Lines=%d-%d\n", i, f.Name, f.FunctionType, f.Package, f.StartLine, f.EndLine)
	}

	if len(funcs) != 2 {
		t.Errorf("Expected 2 functions, got %d", len(funcs))
	}

	// Helper function
	findFunction := func(name string) *FunctionInfo {
		for _, f := range funcs {
			if f.Name == name {
				return &f
			}
		}
		return nil
	}

	// Check main function
	mainFunc := findFunction("main")
	if mainFunc == nil {
		t.Fatal("Function 'main' not found")
	}
	if mainFunc.FunctionType != "function" {
		t.Errorf("Expected function type 'function', got '%s'", mainFunc.FunctionType)
	}
	if mainFunc.StartLine != 10 {
		t.Errorf("Expected start line 10, got %d", mainFunc.StartLine)
	}
	if mainFunc.EndLine != 12 {
		t.Errorf("Expected end line 12, got %d", mainFunc.EndLine)
	}

	foundRunApp := false
	for _, call := range mainFunc.Calls {
		if call == "runApp" {
			foundRunApp = true
			break
		}
	}
	if !foundRunApp {
		t.Errorf("Expected 'runApp' call in main, got %v", mainFunc.Calls)
	}

	// Check build method
	buildFunc := findFunction("build")
	if buildFunc == nil {
		t.Fatal("Method 'build' not found")
	}

	if buildFunc.Package != "MyApp" {
		t.Errorf("Expected Package 'MyApp', got '%s'", buildFunc.Package)
	}

	if buildFunc.StartLine != 5 {
		t.Errorf("Expected start line 5, got %d", buildFunc.StartLine)
	}
	if buildFunc.EndLine != 7 {
		t.Errorf("Expected end line 7, got %d", buildFunc.EndLine)
	}
}
