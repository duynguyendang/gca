package ingest

import "strings"

// isStdLibCall checks if a function call is a known standard library call.
func isStdLibCall(callee string, lang string) bool {
	switch lang {
	case "go":
		// Check for common Go stdlib packages prefix
		parts := strings.Split(callee, ".")
		if len(parts) > 1 {
			pkg := parts[0]
			switch pkg {
			case "fmt", "log", "os", "strings", "strconv", "time", "sync", "math", "errors", "reflect", "io", "context", "bytes", "bufio", "flag", "net", "http", "json", "path", "filepath":
				return true
			}
		}
		// Built-ins
		switch callee {
		case "panic", "append", "len", "cap", "make", "new", "copy", "close", "delete":
			return true
		}
	case "python":
		switch callee {
		case "print", "len", "str", "int", "float", "bool", "list", "dict", "set", "tuple", "range", "open", "type", "isinstance", "enumerate", "zip", "map", "filter", "sum", "min", "max", "abs":
			return true
		}
	case "js":
		if strings.HasPrefix(callee, "console.") || strings.HasPrefix(callee, "Math.") || strings.HasPrefix(callee, "JSON.") {
			return true
		}
		// Common globals
		switch callee {
		case "require", "setTimeout", "setInterval", "clearTimeout", "clearInterval", "parseInt", "parseFloat", "encodeURIComponent", "decodeURIComponent":
			return true
		}
	}
	return false
}
