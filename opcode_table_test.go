package m68kemu

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var updateOpcodeSnapshot = flag.Bool("update-opcodes", false, "rewrite testdata/opcodes_*.txt from the current tables")

// opcodeSnapshot renders an opcode table as one line per run of consecutive
// opcode words that share a handler and a cycle count, so a change to any
// single word shows up as a readable diff.
func opcodeSnapshot(handlers *[0x10000]instruction, cycles *[0x10000]uint32) string {
	name := func(h instruction) string {
		if h == nil {
			return "-"
		}
		n := runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
		return strings.TrimPrefix(n, "github.com/jenska/m68kemu.")
	}
	var b strings.Builder
	start := 0
	for op := 1; op <= len(handlers); op++ {
		if op < len(handlers) && name(handlers[op]) == name(handlers[start]) && cycles[op] == cycles[start] {
			continue
		}
		fmt.Fprintf(&b, "%04x-%04x %s %d\n", start, op-1, name(handlers[start]), cycles[start])
		start = op
	}
	return b.String()
}

func TestOpcodeTableSnapshot68000(t *testing.T) {
	ops := opcodesFor(M68000)
	got := opcodeSnapshot(&ops.handlers, &ops.cycles)

	const path = "testdata/opcodes_68000.txt"
	if *updateOpcodeSnapshot {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TestOpcodeTableSnapshot68000 -update-opcodes to create it)", err)
	}
	if got != string(want) {
		gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range min(len(gotLines), len(wantLines)) {
			if gotLines[i] != wantLines[i] {
				t.Fatalf("opcode table differs from %s at line %d:\n got  %s\n want %s", path, i+1, gotLines[i], wantLines[i])
			}
		}
		t.Fatalf("opcode table differs from %s: %d lines, want %d", path, len(gotLines), len(wantLines))
	}
}

func TestCPUsOfOneModelShareOpcodeSet(t *testing.T) {
	a, _ := newEnvironment(t)
	b, _ := newEnvironment(t)
	if a.ops != b.ops {
		t.Fatalf("two 68000 CPUs have different opcode sets")
	}
	if a.ops != opcodesFor(M68000) {
		t.Fatalf("CPU does not use opcodesFor(M68000)")
	}
}

// TestOpcodesForConcurrentBuild runs under -race: concurrent first calls must
// build the table once and all return it. It uses M68060 because no other
// test builds that table.
func TestOpcodesForConcurrentBuild(t *testing.T) {
	const n = 8
	sets := make([]*opcodeSet, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { sets[i] = opcodesFor(M68060) })
	}
	wg.Wait()
	for i, s := range sets {
		if s == nil || s != sets[0] {
			t.Fatalf("goroutine %d got opcode set %p, goroutine 0 got %p", i, s, sets[0])
		}
	}
}
