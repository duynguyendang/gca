package service

import (
	"testing"
)

func TestInterfacePattern_Match(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   bool
	}{
		{"interface in name", "UserRepository", true},
		{"handler in name", "AuthHandler", true},
		{"service in name", "PaymentService", true},
		{"controller in name", "HomeController", true},
		{"provider in name", "ConfigProvider", true},
		{"client in name", "HTTPClient", true},
		{"adapter in name", "JSONAdapter", true},
		{"factory in name", "WidgetFactory", true},
		{"strategy in name", "PricingStrategy", true},
		{"middleware in name", "LoggingMiddleware", true},
		{"builder in name", "QueryBuilder", true},
		{"parser in name", "XMLParser", true},
		{"validator in name", "InputValidator", true},
		{"not an interface", "MyClass", false},
		{"not an interface 2", "RegularFunction", false},
		{"empty string", "", false},
		{"lowercase interface", "myinterface", true},
		{"mixed case", "SomeInterface", true},
	}

	p := defaultInterfacePattern
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Match(tt.symbol)
			if got != tt.want {
				t.Errorf("InterfacePattern.Match(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
		})
	}
}

func TestIsInterfacePattern(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   bool
	}{
		{"interface keyword", "SomeInterface", true},
		{"handler keyword", "RequestHandler", true},
		{"service keyword", "UserService", true},
		{"not an interface", "MyStruct", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsInterfacePattern(tt.symbol)
			if got != tt.want {
				t.Errorf("IsInterfacePattern(%q) = %v, want %v", tt.symbol, got, tt.want)
			}
		})
	}
}
