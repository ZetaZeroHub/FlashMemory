package parser

import (
	"strings"
)

// countFunctionLines estimates the number of lines in a function based on language-specific patterns
func countFunctionLines(lines []string, startIdx int, lang string) int {
	count := 1 // Start with 1 for the function declaration line
	indentLevel := 0
	//indentChar := " "
	baseIndent := 0

	switch lang {
	case "python":
		// For Python, count indented lines after the function declaration
		if startIdx+1 >= len(lines) {
			return count
		}

		// Determine the base indentation level of the function body
		baseIndent = countLeadingSpaces(lines[startIdx+1])

		for i := startIdx + 1; i < len(lines); i++ {
			line := lines[i]
			if line == "" {
				continue // Skip empty lines
			}

			indent := countLeadingSpaces(line)
			if indent <= 0 || indent < baseIndent {
				break // End of function when indentation returns to previous level
			}
			count++
		}

	case "javascript", "java", "cpp":
		// For brace languages, count lines between opening and closing braces
		for i := startIdx; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue // Skip empty lines but still count them
			}

			// Count opening braces
			for _, char := range line {
				if char == '{' {
					indentLevel++
				} else if char == '}' {
					indentLevel--
					if indentLevel <= 0 && i > startIdx {
						// We've reached the end of the function
						return i - startIdx + 1
					}
				}
			}
		}
	}

	return count
}

// countLeadingSpaces counts the number of leading spaces in a string
func countLeadingSpaces(s string) int {
	count := 0
	for _, char := range s {
		if char == ' ' {
			count++
		} else if char == '\t' {
			// Count tabs as 4 spaces (common convention)
			count += 4
		} else {
			break
		}
	}
	return count
}
