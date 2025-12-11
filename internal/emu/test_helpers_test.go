package emu

import (
	"errors"
	"testing"
)

func expectBusError(t *testing.T, err error) {
	t.Helper()
	var be BusError
	if err == nil || !errors.As(err, &be) {
		t.Fatalf("expected BusError, got %v", err)
	}
}
