package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/tarik02-org/agx/internal/cli/ui"
)

// signatureTypes are the TypeScript type names tool signatures use.
var signatureTypes = map[string]bool{
	"string": true, "number": true, "boolean": true, "unknown": true, "any": true,
	"object": true, "never": true, "void": true, "Record": true,
}

// highlightSignature styles a TypeScript-like tool signature: the tool name, parameter names,
// type names, literals, and punctuation.
func highlightSignature(out *ui.Printer, signature string) string {
	if !out.Styled() {
		return signature
	}
	var b strings.Builder
	runes := []rune(signature)
	named := false
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == '"':
			end := closingQuote(runes, i)
			b.WriteString(out.Code(ui.SyntaxString, string(runes[i:end])))
			i = end
		case unicode.IsDigit(r) || r == '-' && i+1 < len(runes) && unicode.IsDigit(runes[i+1]):
			end := i + 1
			for end < len(runes) && (unicode.IsDigit(runes[end]) || runes[end] == '.') {
				end++
			}
			b.WriteString(out.Code(ui.SyntaxNumber, string(runes[i:end])))
			i = end
		case r == '_' || r == '$' || unicode.IsLetter(r):
			end := i + 1
			for end < len(runes) && (runes[end] == '_' || runes[end] == '$' || runes[end] == '-' ||
				unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end])) {
				end++
			}
			word := string(runes[i:end])
			next := end
			for next < len(runes) && runes[next] == '?' {
				next++
			}
			switch {
			case !named:
				b.WriteString(out.Code(ui.SyntaxName, word))
				named = true
			case next < len(runes) && runes[next] == ':':
				b.WriteString(out.Code(ui.SyntaxKey, word))
			case word == "true" || word == "false" || word == "null" || word == "undefined":
				b.WriteString(out.Code(ui.SyntaxLiteral, word))
			case signatureTypes[word]:
				b.WriteString(out.Code(ui.SyntaxType, word))
			default:
				b.WriteString(word)
			}
			i = end
		case unicode.IsSpace(r):
			b.WriteRune(r)
			i++
		default:
			b.WriteString(out.Code(ui.SyntaxPunctuation, string(r)))
			i++
		}
	}
	return b.String()
}

// closingQuote returns the index after the string literal that starts at runes[start].
func closingQuote(runes []rune, start int) int {
	for i := start + 1; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(runes)
}

// highlightJSON indents and styles text that is a JSON object or array, for a terminal. Other
// text, and any text when output is not styled, is returned unchanged.
func highlightJSON(out *ui.Printer, text string) string {
	trimmed := strings.TrimSpace(text)
	if !out.Styled() || trimmed == "" || trimmed[0] != '{' && trimmed[0] != '[' || !json.Valid([]byte(trimmed)) {
		return text
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(trimmed), "", "  "); err != nil {
		return text
	}
	runes := []rune(indented.String())
	var b strings.Builder
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == '"':
			end := closingQuote(runes, i)
			role := ui.SyntaxString
			if next := skipSpace(runes, end); next < len(runes) && runes[next] == ':' {
				role = ui.SyntaxKey
			}
			b.WriteString(out.Code(role, string(runes[i:end])))
			i = end
		case r == '-' || unicode.IsDigit(r):
			end := i + 1
			for end < len(runes) && strings.ContainsRune("0123456789.eE+-", runes[end]) {
				end++
			}
			b.WriteString(out.Code(ui.SyntaxNumber, string(runes[i:end])))
			i = end
		case unicode.IsLetter(r):
			end := i + 1
			for end < len(runes) && unicode.IsLetter(runes[end]) {
				end++
			}
			b.WriteString(out.Code(ui.SyntaxLiteral, string(runes[i:end])))
			i = end
		case unicode.IsSpace(r):
			b.WriteRune(r)
			i++
		default:
			b.WriteString(out.Code(ui.SyntaxPunctuation, string(r)))
			i++
		}
	}
	return b.String()
}

func skipSpace(runes []rune, i int) int {
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	return i
}
