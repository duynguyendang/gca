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
			case "fmt", "log", "os", "strings", "strconv", "time", "sync", "math", "errors", "reflect", "io", "context", "bytes", "bufio", "flag", "net", "http", "json", "path", "filepath", "sort", "container", "crypto", "encoding", "html", "image", "index", "mime", "runtime", "testing", "text", "unicode":
				return true
			}
		}
		// Built-ins
		switch callee {
		case "panic", "append", "len", "cap", "make", "new", "copy", "close", "delete", "recover", "real", "imag", "complex":
			return true
		}
	case "python":
		switch callee {
		case "print", "len", "str", "int", "float", "bool", "list", "dict", "set", "tuple", "range", "open", "type", "isinstance", "enumerate", "zip", "map", "filter", "sum", "min", "max", "abs", "any", "all", "sorted", "reversed", "dir", "help", "vars", "getattr", "setattr", "hasattr":
			return true
		}
	case "js":
		if strings.HasPrefix(callee, "console.") || strings.HasPrefix(callee, "Math.") || strings.HasPrefix(callee, "JSON.") || strings.HasPrefix(callee, "Reflect.") || strings.HasPrefix(callee, "Proxy.") || strings.HasPrefix(callee, "Intl.") {
			return true
		}
		// Common globals (Browser + Node)
		switch callee {
		case "window", "document", "navigator", "location", "history", "localStorage", "sessionStorage", "fetch", "XMLHttpRequest", "Promise", "Object", "Array", "String", "Number", "Boolean", "RegExp", "Error", "Map", "Set", "WeakMap", "WeakSet", "process", "require", "module", "exports", "__dirname", "__filename", "setTimeout", "setInterval", "clearTimeout", "clearInterval", "parseInt", "parseFloat", "encodeURIComponent", "decodeURIComponent":
			return true
		}
	}
	return false
}
