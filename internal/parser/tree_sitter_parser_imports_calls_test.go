package parser

import (
	"os"
	"reflect"
	"sort"
	"testing"
)

// TestExtractImports_Go 测试Go语言导入提取功能
func TestExtractImports_Go(t *testing.T) {
	code := `package test

import (
	"fmt"
	"strings"
	"github.com/example/pkg"
)

func Test() {}
`
	fileName := createTempFile(t, "go", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "go"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Go代码解析失败: %v", err)
	}

	// 验证导入信息
	if len(funcs) == 0 {
		t.Fatal("未解析到任何函数")
	}

	expectedImports := []string{"fmt", "strings", "github.com/example/pkg"}
	sort.Strings(expectedImports)
	actualImports := funcs[0].Imports
	sort.Strings(actualImports)

	if !reflect.DeepEqual(expectedImports, actualImports) {
		t.Errorf("导入解析错误，期望: %v, 实际: %v", expectedImports, actualImports)
	}
}

// TestExtractImports_Python 测试Python语言导入提取功能
func TestExtractImports_Python(t *testing.T) {
	code := `# Python test
import os
import sys
from datetime import datetime

def test():
    pass
`
	fileName := createTempFile(t, "py", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "python"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Python代码解析失败: %v", err)
	}

	// 验证导入信息
	if len(funcs) == 0 {
		t.Fatal("未解析到任何函数")
	}

	expectedImports := []string{"os", "sys", "datetime"}
	sort.Strings(expectedImports)
	actualImports := funcs[0].Imports
	sort.Strings(actualImports)

	if !reflect.DeepEqual(expectedImports, actualImports) {
		t.Errorf("导入解析错误，期望: %v, 实际: %v", expectedImports, actualImports)
	}
}

// TestExtractImports_JavaScript 测试JavaScript语言导入提取功能
func TestExtractImports_JavaScript(t *testing.T) {
	code := `// JavaScript test
import { useState } from 'react';
import axios from 'axios';

function test() {}
`
	fileName := createTempFile(t, "js", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "javascript"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("JavaScript代码解析失败: %v", err)
	}

	// 验证导入信息
	if len(funcs) == 0 {
		t.Fatal("未解析到任何函数")
	}

	expectedImports := []string{"react", "axios"}
	sort.Strings(expectedImports)
	actualImports := funcs[0].Imports
	sort.Strings(actualImports)

	if !reflect.DeepEqual(expectedImports, actualImports) {
		t.Errorf("导入解析错误，期望: %v, 实际: %v", expectedImports, actualImports)
	}
}

// TestExtractImports_Java 测试Java语言导入提取功能
func TestExtractImports_Java(t *testing.T) {
	code := `package test;

import java.util.List;
import java.util.ArrayList;
import org.example.Test;

public class TestClass {
    public void test() {}
}
`
	fileName := createTempFile(t, "java", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "java"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Java代码解析失败: %v", err)
	}

	// 验证导入信息
	if len(funcs) == 0 {
		t.Fatal("未解析到任何函数")
	}

	expectedImports := []string{"java.util.List", "java.util.ArrayList", "org.example.Test"}
	sort.Strings(expectedImports)
	actualImports := funcs[0].Imports
	sort.Strings(actualImports)

	if !reflect.DeepEqual(expectedImports, actualImports) {
		t.Errorf("导入解析错误，期望: %v, 实际: %v", expectedImports, actualImports)
	}
}

// TestExtractImports_Rust 测试Rust语言导入提取功能
func TestExtractImports_Rust(t *testing.T) {
	code := `// Rust test
use std::io;
use std::collections::HashMap;

fn test() {}
`
	fileName := createTempFile(t, "rs", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "rust"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Rust代码解析失败: %v", err)
	}

	// 验证导入信息
	if len(funcs) == 0 {
		t.Fatal("未解析到任何函数")
	}

	expectedImports := []string{"std::io", "std::collections::HashMap"}
	sort.Strings(expectedImports)
	actualImports := funcs[0].Imports
	sort.Strings(actualImports)

	if !reflect.DeepEqual(expectedImports, actualImports) {
		t.Errorf("导入解析错误，期望: %v, 实际: %v", expectedImports, actualImports)
	}
}

// TestExtractImports_PHP 测试PHP语言导入提取功能
func TestExtractImports_PHP(t *testing.T) {
	code := `<?php
namespace Test;

use App\Models\User;
use App\Services\Auth;

function test() {}
?>`
	fileName := createTempFile(t, "php", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "php"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("PHP代码解析失败: %v", err)
	}

	// 验证导入信息
	if len(funcs) == 0 {
		t.Fatal("未解析到任何函数")
	}

	expectedImports := []string{"App\\Models\\User", "App\\Services\\Auth"}
	sort.Strings(expectedImports)
	actualImports := funcs[0].Imports
	sort.Strings(actualImports)

	if !reflect.DeepEqual(expectedImports, actualImports) {
		t.Errorf("导入解析错误，期望: %v, 实际: %v", expectedImports, actualImports)
	}
}

// TestExtractCalls_Go 测试Go语言函数调用提取功能
func TestExtractCalls_Go(t *testing.T) {
	code := `package test

import "fmt"

func helper1() {}
func helper2() {}

func Test() {
	fmt.Println("test")
	helper1()
	helper2()
}
`
	fileName := createTempFile(t, "go", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "go"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Go代码解析失败: %v", err)
	}

	// 查找Test函数
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "Test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到Test函数")
	}

	// 验证函数调用
	expectedCalls := []string{"Println", "helper1", "helper2"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// TestExtractCalls_Python 测试Python语言函数调用提取功能
func TestExtractCalls_Python(t *testing.T) {
	code := `# Python test
def helper1():
    pass

def helper2():
    pass

def test():
    print("test")
    helper1()
    helper2()
    obj.method()
`
	fileName := createTempFile(t, "py", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "python"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Python代码解析失败: %v", err)
	}

	// 查找test函数
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到test函数")
	}

	// 验证函数调用
	expectedCalls := []string{"print", "helper1", "helper2", "method"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// TestExtractCalls_JavaScript 测试JavaScript语言函数调用提取功能
func TestExtractCalls_JavaScript(t *testing.T) {
	code := `// JavaScript test
function helper1() {}
function helper2() {}

function test() {
    console.log("test");
    helper1();
    helper2();
    obj.method();
}
`
	fileName := createTempFile(t, "js", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "javascript"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("JavaScript代码解析失败: %v", err)
	}

	// 查找test函数
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到test函数")
	}

	// 验证函数调用
	expectedCalls := []string{"log", "helper1", "helper2", "method"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// TestExtractCalls_Java 测试Java语言函数调用提取功能
func TestExtractCalls_Java(t *testing.T) {
	code := `package test;

public class TestClass {
    public void helper1() {}
    public void helper2() {}
    
    public void test() {
        System.out.println("test");
        helper1();
        helper2();
    }
}
`
	fileName := createTempFile(t, "java", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "java"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Java代码解析失败: %v", err)
	}

	// 查找test方法
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到test方法")
	}

	// 验证函数调用
	expectedCalls := []string{"println", "helper1", "helper2"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// TestExtractCalls_Cpp 测试C++语言函数调用提取功能
func TestExtractCalls_Cpp(t *testing.T) {
	code := `// C++ test
#include <iostream>

void helper1() {}
void helper2() {}

void test() {
    std::cout << "test" << std::endl;
    helper1();
    helper2();
}
`
	fileName := createTempFile(t, "cpp", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "cpp"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("C++代码解析失败: %v", err)
	}

	// 查找test函数
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到test函数")
	}

	// 验证函数调用
	expectedCalls := []string{"helper1", "helper2"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// TestExtractCalls_Rust 测试Rust语言函数调用提取功能
func TestExtractCalls_Rust(t *testing.T) {
	code := `// Rust test
fn helper1() {}
fn helper2() {}

fn test() {
    println!("test");
    helper1();
    helper2();
    obj.method();
}
`
	fileName := createTempFile(t, "rs", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "rust"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("Rust代码解析失败: %v", err)
	}

	// 查找test函数
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到test函数")
	}

	// 验证函数调用
	expectedCalls := []string{"helper1", "helper2", "method"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// TestExtractCalls_PHP 测试PHP语言函数调用提取功能
func TestExtractCalls_PHP(t *testing.T) {
	code := `<?php
function helper1() {}
function helper2() {}

function test() {
    echo "test";
    helper1();
    helper2();
    $obj->method();
}
?>`
	fileName := createTempFile(t, "php", code)
	defer removeFile(t, fileName)

	parser := &TreeSitterParser{Lang: "php"}
	funcs, err := parser.ParseFile(fileName)
	if err != nil {
		t.Fatalf("PHP代码解析失败: %v", err)
	}

	// 查找test函数
	var testFunc FunctionInfo
	for _, f := range funcs {
		if f.Name == "test" {
			testFunc = f
			break
		}
	}

	if testFunc.Name == "" {
		t.Fatal("未找到test函数")
	}

	// 验证函数调用
	expectedCalls := []string{"helper1", "helper2", "method"}
	sort.Strings(expectedCalls)
	actualCalls := testFunc.Calls
	sort.Strings(actualCalls)

	if !reflect.DeepEqual(expectedCalls, actualCalls) {
		t.Errorf("函数调用解析错误，期望: %v, 实际: %v", expectedCalls, actualCalls)
	}
}

// 辅助函数：删除文件
func removeFile(t *testing.T, path string) {
	if err := os.Remove(path); err != nil {
		t.Logf("删除临时文件失败: %v", err)
	}
}
