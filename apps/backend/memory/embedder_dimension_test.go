package memory

import (
	"strings"
	"testing"
)

func TestNewOpenAIEmbedderRejectsUnsupportedDimension(t *testing.T) {
	t.Parallel()
	if _, err := NewOpenAIEmbedder("http://localhost", "key", "text-embedding-3-large"); err == nil {
		t.Fatal("expected rejection of a 3072-dimension (large) model")
	} else if !strings.Contains(err.Error(), "vector(1536)") {
		t.Errorf("error should name the supported dimension; got %v", err)
	}

	e, err := NewOpenAIEmbedder("http://localhost", "key", "text-embedding-3-small")
	if err != nil {
		t.Fatalf("default-dimension model must be accepted: %v", err)
	}
	if e.Dimension() != SupportedEmbeddingDimension {
		t.Errorf("Dimension() = %d, want %d", e.Dimension(), SupportedEmbeddingDimension)
	}
}
