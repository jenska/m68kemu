package main

import (
	"fmt"
	"log"

	asm "github.com/jenska/m68kasm"
	m68kemu "github.com/jenska/m68kemu"
)

const (
	stackPointer = 0x8000
	startAddress = 0x2000
)

func main() {
	program, err := asm.AssembleFile("testdata/qsort.s")
	if err != nil {
		log.Fatalf("failed to assemble qsort.s: %v", err)
	}

	ram := m68kemu.NewRAM(0, 0x10000)
	bus := m68kemu.NewBus(&ram)

	if err := ram.Write(m68kemu.Long, 0, stackPointer); err != nil {
		log.Fatalf("failed to write stack pointer: %v", err)
	}
	if err := ram.Write(m68kemu.Long, 4, startAddress); err != nil {
		log.Fatalf("failed to write start address: %v", err)
	}

	for i, b := range program {
		address := startAddress + uint32(i)
		if err := ram.Write(m68kemu.Byte, address, uint32(b)); err != nil {
			log.Fatalf("failed to write program byte at %04x: %v", address, err)
		}
	}

	cpu, err := m68kemu.NewCPU(bus)
	if err != nil {
		log.Fatalf("failed to create CPU: %v", err)
	}

	var lastPC uint32
	var steps int
	for steps = 0; steps < 100000; steps++ {
		lastPC = cpu.Registers().PC
		if err := cpu.Step(); err != nil {
			log.Fatalf("execution failed at PC %04x: %v", lastPC, err)
		}
		if cpu.Registers().PC == lastPC {
			break // reached the BRA halt loop
		}
	}
	if steps == 100000 {
		log.Fatalf("quicksort did not reach the halt loop; PC=%04x", cpu.Registers().PC)
	}

	base := cpu.Registers().A[0]
	fmt.Printf("Sorted array at 0x%04x:\n", base)
	for i := 0; i < 10; i++ {
		value, err := ram.Read(m68kemu.Long, base+uint32(i*4))
		if err != nil {
			log.Fatalf("failed to read sorted value %d: %v", i, err)
		}
		fmt.Printf("a[%d] = %d\n", i, int32(value))
	}
	fmt.Printf("Completed in %d instructions (%d cycles)\n", steps+1, cpu.Cycles())
}
