package cli

import (
	"errors"
	"testing"
)

func TestExitError(t *testing.T) {
	plain := &ExitError{Code: 42}
	if plain.Error() != "" {
		t.Fatalf("empty wrap should print nothing, got %q", plain.Error())
	}

	wrapped := &ExitError{Code: 3, Err: errors.New("boom")}
	if wrapped.Error() != "boom" {
		t.Fatalf("Error() = %q", wrapped.Error())
	}
	if !errors.Is(wrapped, wrapped.Err) {
		t.Fatal("Unwrap failed")
	}
}
