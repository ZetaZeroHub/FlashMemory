package module_analyzer

import (
	"fmt"
	"strings"

	"github.com/kinglegendzzh/flashmemory/config"
	"github.com/kinglegendzzh/flashmemory/internal/analyzer"
)

// PromptSelector decides which prompt template (code-oriented vs.
// document-oriented vs. mixed) to feed into the LLM when summarizing a file
// or directory. It exists to fix BUG-001: documents (FunctionType =
// "llm_parser") were previously routed through code-oriented prompts, leading
// to "this file implements a function named …" hallucinations and a real
// prompt-injection vector via knowledge-base content.
type PromptSelector struct {
	cfg *config.Config

	// DocRatioThreshold — fraction of llm_parser entries above which the file
	// is treated as documentation. Default 0.95 (≥95% docs → DocFile prompt).
	DocRatioThreshold float64

	// MixedThreshold — fraction below which the file is treated as code-only
	// (default 0.05). Between the two thresholds it's a mixed file.
	MixedThreshold float64
}

// PromptKind enumerates the supported prompt strategies.
type PromptKind int

const (
	PromptKindCode  PromptKind = iota // FileAnalyzerPrompts (legacy, code-only)
	PromptKindDoc                     // DocFileAnalyzerPrompts (knowledge-base file)
	PromptKindMixed                   // both, with explicit grouping
)

func NewPromptSelector(cfg *config.Config) *PromptSelector {
	return &PromptSelector{cfg: cfg, DocRatioThreshold: 0.95, MixedThreshold: 0.05}
}

// SelectFilePromptKind returns which prompt template is appropriate for a
// file given the FunctionType distribution of its parsed entries.
func (s *PromptSelector) SelectFilePromptKind(funcs []analyzer.LLMAnalysisResult) PromptKind {
	if len(funcs) == 0 {
		return PromptKindCode
	}
	docCount := 0
	for _, f := range funcs {
		if f.Func.FunctionType == "llm_parser" {
			docCount++
		}
	}
	ratio := float64(docCount) / float64(len(funcs))
	switch {
	case ratio >= s.DocRatioThreshold:
		return PromptKindDoc
	case ratio <= s.MixedThreshold:
		return PromptKindCode
	default:
		return PromptKindMixed
	}
}

// SelectDirectoryPromptKind decides whether the directory is doc-heavy.
// When >= 70% of immediate children are doc-only files, use DocModule prompts.
func (s *PromptSelector) SelectDirectoryPromptKind(docFileCount, totalFileCount int) PromptKind {
	if totalFileCount == 0 {
		return PromptKindCode
	}
	ratio := float64(docFileCount) / float64(totalFileCount)
	if ratio >= 0.7 {
		return PromptKindDoc
	}
	return PromptKindCode
}

// SandboxDocContent wraps untrusted document text in fenced code blocks so the
// upstream LLM cannot interpret embedded instructions as new commands.
//
// Three reasons:
//  1. Knowledge-base content can include "ignore previous instructions"-style
//     injection probes (the hackathon Trap subtype prompt_injection deliberately
//     ships these).
//  2. Triple-backtick fencing is the canonical "code block" hint in markdown
//     and most LLMs treat it as data, not instructions.
//  3. We strip any pre-existing "```" sequences from the inner text so an
//     attacker cannot break out of the fence by embedding their own.
func SandboxDocContent(content string) string {
	cleaned := strings.ReplaceAll(content, "```", "ʼʼʼ")
	return fmt.Sprintf("```text\n%s\n```", cleaned)
}

// FormatDocFilePrompt builds the prompt for an all-documents file. Sections
// are listed without "function/method" terminology; their content is
// sandboxed before injection.
func (s *PromptSelector) FormatDocFilePrompt(modulePath string, sections []analyzer.LLMAnalysisResult) string {
	p := s.cfg.DocFileAnalyzerPrompts
	header := orFallback(p.Header, "请为以下知识库文档生成一个简洁的内容摘要。文档路径:")
	subHeader := orFallback(p.SubModuleHeader, "文档中包含的章节及其内容摘要:")
	footer := orFallback(p.Footer,
		"请基于以上章节内容，生成一个简洁的文档级摘要，包括：\n"+
			"1. 该文档涵盖的主要主题/业务领域\n"+
			"2. 文档结构（多少章节、覆盖哪些子主题）\n"+
			"3. 该文档对回答用户问题的参考价值\n"+
			"注意：这是一份业务文档（非代码），请勿使用\"模块/类/函数\"等代码术语。")

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n\n%s\n\n", header, modulePath, subHeader)
	for _, f := range sections {
		title := f.Func.Name
		desc := strings.TrimSpace(f.Description)
		if desc == "" {
			desc = strings.TrimSpace(f.CodeSnippet)
		}
		// Sandbox each chunk so injected instructions can't escape.
		sb.WriteString(fmt.Sprintf("- 章节 %s:\n%s\n\n", title, SandboxDocContent(desc)))
	}
	sb.WriteString(footer)
	return sb.String()
}

// FormatMixedFilePrompt produces a prompt that explicitly groups code units
// from document sections, giving the LLM clear scope for each block.
func (s *PromptSelector) FormatMixedFilePrompt(modulePath string, funcs []analyzer.LLMAnalysisResult) string {
	var codeFns, docSecs []analyzer.LLMAnalysisResult
	for _, f := range funcs {
		if f.Func.FunctionType == "llm_parser" {
			docSecs = append(docSecs, f)
		} else {
			codeFns = append(codeFns, f)
		}
	}

	p := s.cfg.FileAnalyzerPrompts
	header := orFallback(p.Header, "请为以下文件生成一个全面的模块级描述。文件路径:")

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n\n", header, modulePath)

	if len(codeFns) > 0 {
		sb.WriteString("代码单元:\n")
		for _, f := range codeFns {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", f.Func.Name, f.Description))
		}
		sb.WriteString("\n")
	}
	if len(docSecs) > 0 {
		sb.WriteString("文档章节:\n")
		for _, f := range docSecs {
			desc := strings.TrimSpace(f.Description)
			if desc == "" {
				desc = strings.TrimSpace(f.CodeSnippet)
			}
			sb.WriteString(fmt.Sprintf("- %s:\n%s\n\n", f.Func.Name, SandboxDocContent(desc)))
		}
	}

	footer := orFallback(p.Footer, "请基于以上内容生成文件级描述，分别说明代码部分和文档部分的作用。")
	sb.WriteString(footer)
	return sb.String()
}

func orFallback(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
