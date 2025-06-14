package main

import (
	"fmt"
	"strings"
)

// SimpleFunction is a basic function for testing the parser
func SimpleFunction() {
	fmt.Println("Hello, world!")
}

// FunctionWithParams demonstrates a function with parameters
func FunctionWithParams(name string, age int) string {
	return fmt.Sprintf("Hello, %s! You are %d years old.", name, age)
}

// StructWithMethods demonstrates a struct with methods
type StructWithMethods struct {
	Name string
	Age  int
}

// GetInfo is a method on StructWithMethods
func (s *StructWithMethods) GetInfo() string {
	return fmt.Sprintf("%s is %d years old", s.Name, s.Age)
}

// UpdateAge updates the age and calls other functions
func (s *StructWithMethods) UpdateAge(newAge int) {
	s.Age = newAge
	SimpleFunction()
	FunctionWithParams(s.Name, s.Age)
	s.privateHelper()
}

// privateHelper is a private helper method
func (s *StructWithMethods) privateHelper() {
	strings.ToUpper(s.Name)
}

func main() {
	person := &StructWithMethods{Name: "Alice", Age: 30}
	fmt.Println(person.GetInfo())
	person.UpdateAge(31)
}
