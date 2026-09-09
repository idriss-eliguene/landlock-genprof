package proposalapp

import (
	"context"
	"testing"
)

func TestGenerateRequiresItsApplicationDependencies(t *testing.T) {
	if _, err := Generate(context.Background(), nil, "default", "observation", "proposal", nil); err == nil {
		t.Fatal("Generate accepted missing dependencies")
	}
}
