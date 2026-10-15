package templates

import (
	"os"
	"testing"
)

func TestNewParsesTemplates(t *testing.T) {
	if _, err := New(os.DirFS("../..")); err != nil {
		t.Fatalf("parse templates: %v", err)
	}
}
