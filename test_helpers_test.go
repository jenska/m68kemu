package m68kemu

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

func expectAddressError(t *testing.T, err error) {
	t.Helper()
	var ae AddressError
	if err == nil || !errors.As(err, &ae) {
		t.Fatalf("expected BusError, got %v", err)
	}
}
