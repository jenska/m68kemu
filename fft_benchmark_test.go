package m68kemu_test

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	asm "github.com/jenska/m68kasm"
	m68kemu "github.com/jenska/m68kemu"
	"github.com/jenska/m68kemu/fpu"
)

// The FFT benchmarks run the same in-place radix-2 decimation-in-time FFT in
// four arithmetic flavours, each on the CPU models that can execute it:
//
//   - Q15: 16-bit fixed point with MULS.W, the classic 68000 approach.
//   - Q31: 32-bit fixed point with the 68020's 32x32->64 MULS.L.
//   - SFP004: double precision on a 68000 driving a memory-mapped MC68881
//     through its coprocessor interface registers, as on the Atari Mega ST.
//   - FPU: double precision with native F-line instructions on a 68020 or
//     68030 with an MC68881/MC68882 coprocessor.
//
// Every program shares the same skeleton: a Gold-Rader bit reversal followed
// by log2(N) butterfly stages over a precomputed twiddle table. Only the
// butterfly body and the data formats differ, so instruction and cycle counts
// compare the arithmetic, not the loop structure.
//
// Besides ns/op the benchmarks report the emulated work per FFT (instrs/fft,
// cycles/fft), the emulator throughput (Minstr/s) and the accuracy against a
// float64 reference (SNR-dB). The FPU runs at the CPU's clock with the
// typical execution times of package fpu; integer instructions overlap it, and
// on the SFP004 the 68000 polls while it is busy. From the 68020 on, integer
// cycle counts reuse the 68000 timing tables, so they are an approximation;
// instruction counts are exact, apart from the SFP004's polling loops, which
// depend on the FPU timing.

const (
	fftBenchSize  = 1024
	fftCodeBase   = 0x2000
	fftDataBase   = 0x10000
	fftTwBase     = 0x20000
	fftRAMSize    = 0x40000
	fftCIRBase    = 0xfffa40
	fftCIROperand = fftCIRBase + 0x10
)

// fftMachine is one CPU configuration a variant runs on.
type fftMachine struct {
	model m68kemu.Model
	fpu   fpu.Model
	// coprocessor attaches the FPU through the 68020 coprocessor interface,
	// cir maps it at $FFFA40 for a 68000 or 68010. Neither: no FPU.
	coprocessor, cir bool
}

func (m fftMachine) String() string {
	switch {
	case m.coprocessor, m.cir:
		return m.model.String() + "+" + [...]string{fpu.MC68881: "MC68881", fpu.MC68882: "MC68882"}[m.fpu]
	default:
		return m.model.String()
	}
}

// fftVariant is one arithmetic flavour of the FFT.
type fftVariant struct {
	name     string
	target   asm.Target
	machines []fftMachine
	// elem and tw are the sizes in bytes of a complex sample and of a
	// twiddle factor.
	elem, tw int
	// prologue sets up registers the butterfly needs; butterfly computes
	// one butterfly (see fftSkeleton for the register contract).
	prologue  string
	butterfly string
	// put stores a complex value in memory format, get reads one back and
	// undoes the variant's scaling.
	put func(buf []byte, v complex128)
	get func(buf []byte, n int) complex128
	// minSNR is the accuracy the variant must reach, in dB.
	minSNR float64
}

// fftSkeleton is the code every variant shares. It halts in the loop at
// "done", 4 bytes past the start. Registers in the butterfly loop:
//
//	A0 = a (start+k), A1 = b (start+k+half), A2 = twiddle, A4 = end of group
//	A5 = twiddle table, A6 = end of data, D6 = half in bytes,
//	D7 = twiddle stride in bytes. D0-D5, FP0-FP7 and A3 are free.
//
// The butterfly must advance A0 and A1 by one sample and A2 by D7.
const fftSkeleton = `
	BRA.W   start
done:	BRA.S   done
start:	LEA     DATA,A0
	MOVEQ   #0,D0             ; i, in bytes
	MOVEQ   #0,D1             ; j, in bytes
	MOVE.W  #N-2,D7
rev:	CMP.L   D1,D0
	BGE.S   noswap
SWAP
noswap:	MOVE.L  #HALF,D2          ; k = N/2 samples, in bytes
kloop:	CMP.L   D2,D1
	BLT.S   kdone
	SUB.L   D2,D1
	LSR.L   #1,D2
	BRA.S   kloop
kdone:	ADD.L   D2,D1
	ADD.L   #ELEM,D0
	DBRA    D7,rev

	MOVEQ   #ELEM,D6
	MOVE.L  #STRIDE,D7
	LEA     TWIDDLES,A5
	LEA     END,A6
PROLOGUE
stage:	LEA     DATA,A0
group:	MOVEA.L A0,A1
	ADDA.L  D6,A1
	MOVEA.L A1,A4
	MOVEA.L A5,A2
inner:
BUTTERFLY
	CMPA.L  A4,A0
	BNE.W   inner
	MOVEA.L A1,A0
	CMPA.L  A6,A0
	BNE.W   group
	ADD.L   D6,D6
	LSR.L   #1,D7
	CMP.L   #TW,D7
	BHS.W   stage
	BRA.W   done
`

const fftDonePC = fftCodeBase + 4

// The fixed-point butterflies halve every stage's output so the values stay
// in range; the result is the FFT divided by N.

const fftQ15Butterfly = `
	MOVE.W  (A2),D0           ; wr
	MOVE.W  2(A2),D1          ; wi
	ADDA.L  D7,A2
	MOVE.W  (A1),D2           ; br
	MOVE.W  2(A1),D3          ; bi
	MOVE.W  D2,D4
	MULS.W  D0,D4             ; br*wr
	MOVE.W  D3,D5
	MULS.W  D1,D5             ; bi*wi
	SUB.L   D5,D4
	MULS.W  D1,D2             ; br*wi
	MULS.W  D0,D3             ; bi*wr
	ADD.L   D3,D2
	SWAP    D4                ; tr/2
	SWAP    D2                ; ti/2
	MOVE.W  (A0),D0
	ASR.W   #1,D0             ; ar/2
	MOVE.W  D0,D1
	ADD.W   D4,D0
	SUB.W   D4,D1
	MOVE.W  2(A0),D3
	ASR.W   #1,D3             ; ai/2
	MOVE.W  D3,D5
	ADD.W   D2,D3
	SUB.W   D2,D5
	MOVE.W  D0,(A0)+
	MOVE.W  D3,(A0)+
	MOVE.W  D1,(A1)+
	MOVE.W  D5,(A1)+
`

// The high long of a Q31 x Q31 product is half the product in Q31.
const fftQ31Butterfly = `
	MOVE.L  (A2),D0           ; wr
	MOVE.L  4(A2),D1          ; wi
	ADDA.L  D7,A2
	MOVE.L  (A1),D2           ; br
	MOVE.L  4(A1),D3          ; bi
	MOVE.L  D2,D4
	MULS.L  D0,D5:D4          ; D5 = br*wr/2
	MULS.L  D3,D4:D0          ; D4 = bi*wr/2
	MULS.L  D1,D0:D2          ; D0 = br*wi/2
	MULS.L  D1,D2:D3          ; D2 = bi*wi/2
	SUB.L   D2,D5             ; tr/2
	ADD.L   D4,D0             ; ti/2
	MOVE.L  (A0),D1
	ASR.L   #1,D1             ; ar/2
	MOVE.L  D1,D2
	ADD.L   D5,D1
	SUB.L   D5,D2
	MOVE.L  4(A0),D3
	ASR.L   #1,D3             ; ai/2
	MOVE.L  D3,D4
	ADD.L   D0,D3
	SUB.L   D0,D4
	MOVE.L  D1,(A0)+
	MOVE.L  D3,(A0)+
	MOVE.L  D2,(A1)+
	MOVE.L  D4,(A1)+
`

// fpOp is one step of the floating-point butterfly, rendered either as an
// F-line instruction or as an SFP004 command sequence.
type fpOp struct {
	kind  byte // 'l' load, 's' store, 'r' register to register, 'i' integer
	op    string
	m, n  int    // source and destination FP register
	ea    string // address register of a load or store
	off   int    // displacement of a load
	asmOp string // integer instruction for kind 'i'
}

// fftFloatButterfly computes the butterfly in FP0-FP6 on doubles in memory.
var fftFloatButterfly = []fpOp{
	{kind: 'l', ea: "A2", off: 0, n: 0}, // wr
	{kind: 'l', ea: "A2", off: 8, n: 1}, // wi
	{kind: 'i', asmOp: "ADDA.L  D7,A2"},
	{kind: 'l', ea: "A1", off: 0, n: 2}, // br
	{kind: 'l', ea: "A1", off: 8, n: 3}, // bi
	{kind: 'r', op: "MOVE", m: 2, n: 4},
	{kind: 'r', op: "MUL", m: 0, n: 4}, // br*wr
	{kind: 'r', op: "MOVE", m: 3, n: 5},
	{kind: 'r', op: "MUL", m: 1, n: 5},  // bi*wi
	{kind: 'r', op: "SUB", m: 5, n: 4},  // tr
	{kind: 'r', op: "MUL", m: 1, n: 2},  // br*wi
	{kind: 'r', op: "MUL", m: 0, n: 3},  // bi*wr
	{kind: 'r', op: "ADD", m: 3, n: 2},  // ti
	{kind: 'l', ea: "A0", off: 0, n: 0}, // ar
	{kind: 'l', ea: "A0", off: 8, n: 1}, // ai
	{kind: 'r', op: "MOVE", m: 0, n: 5},
	{kind: 'r', op: "ADD", m: 4, n: 0},
	{kind: 'r', op: "SUB", m: 4, n: 5},
	{kind: 'r', op: "MOVE", m: 1, n: 6},
	{kind: 'r', op: "ADD", m: 2, n: 1},
	{kind: 'r', op: "SUB", m: 2, n: 6},
	{kind: 's', ea: "A0", n: 0},
	{kind: 's', ea: "A0", n: 1},
	{kind: 's', ea: "A1", n: 5},
	{kind: 's', ea: "A1", n: 6},
}

var fpOpmodes = map[string]int{"MOVE": 0x00, "ADD": 0x22, "MUL": 0x23, "SUB": 0x28}

// fpLineCode renders ops as MC68020 F-line instructions.
func fpLineCode(ops []fpOp) string {
	var sb strings.Builder
	for _, o := range ops {
		switch o.kind {
		case 'l':
			fmt.Fprintf(&sb, "\tFMOVE.D %d(%s),FP%d\n", o.off, o.ea, o.n)
		case 's':
			fmt.Fprintf(&sb, "\tFMOVE.D FP%d,(%s)+\n", o.n, o.ea)
		case 'r':
			fmt.Fprintf(&sb, "\tF%s.X FP%d,FP%d\n", o.op, o.m, o.n)
		case 'i':
			fmt.Fprintf(&sb, "\t%s\n", o.asmOp)
		}
	}
	return sb.String()
}

// fpCIRCode renders ops as the command sequences a 68000 sends to the
// memory-mapped FPU, with A3 pointing to the operand CIR: write the command
// word, wait for the FPU, then move the operand. The waits poll the response
// CIR like the SFP004 library: while it reads $8900 for an operand transfer,
// while bit 15 is set for a register-to-register command.
func fpCIRCode(ops []fpOp) string {
	var sb strings.Builder
	label := 0
	wait := func(test, branch string) {
		label++
		fmt.Fprintf(&sb, "w%d:\t%s\n\t%s w%d\n", label, test, branch, label)
	}
	for _, o := range ops {
		switch o.kind {
		case 'l': // FMOVE.D <ea>,FPn
			fmt.Fprintf(&sb, "\tMOVE.W  #$%04X,-6(A3)\n", 0x5400|o.n<<7)
			wait("CMPI.W  #$8900,-16(A3)", "BEQ.S")
			fmt.Fprintf(&sb, "\tMOVE.L  %d(%s),(A3)\n\tMOVE.L  %d(%s),(A3)\n", o.off, o.ea, o.off+4, o.ea)
		case 's': // FMOVE.D FPn,<ea>
			fmt.Fprintf(&sb, "\tMOVE.W  #$%04X,-6(A3)\n", 0x7400|o.n<<7)
			wait("CMPI.W  #$8900,-16(A3)", "BEQ.S")
			fmt.Fprintf(&sb, "\tMOVE.L  (A3),(%s)+\n\tMOVE.L  (A3),(%s)+\n", o.ea, o.ea)
		case 'r':
			fmt.Fprintf(&sb, "\tMOVE.W  #$%04X,-6(A3)\n", o.m<<10|o.n<<7|fpOpmodes[o.op])
			wait("TST.W   -16(A3)", "BMI.S")
		case 'i':
			fmt.Fprintf(&sb, "\t%s\n", o.asmOp)
		}
	}
	return sb.String()
}

func putQ(buf []byte, v complex128, bits int) {
	scale := math.Ldexp(1, bits-1) - 1
	re, im := math.Round(real(v)*scale), math.Round(imag(v)*scale)
	if bits == 16 {
		binary.BigEndian.PutUint16(buf, uint16(int16(re)))
		binary.BigEndian.PutUint16(buf[2:], uint16(int16(im)))
		return
	}
	binary.BigEndian.PutUint32(buf, uint32(int32(re)))
	binary.BigEndian.PutUint32(buf[4:], uint32(int32(im)))
}

func putDouble(buf []byte, v complex128) {
	binary.BigEndian.PutUint64(buf, math.Float64bits(real(v)))
	binary.BigEndian.PutUint64(buf[8:], math.Float64bits(imag(v)))
}

func getDouble(buf []byte, _ int) complex128 {
	return complex(math.Float64frombits(binary.BigEndian.Uint64(buf)), math.Float64frombits(binary.BigEndian.Uint64(buf[8:])))
}

var (
	fftWithoutFPU = []fftMachine{{model: m68kemu.M68000}, {model: m68kemu.M68010}, {model: m68kemu.M68020}, {model: m68kemu.M68030}}
	fft020        = []fftMachine{{model: m68kemu.M68020}, {model: m68kemu.M68030}}
	fftSFP004     = []fftMachine{{model: m68kemu.M68000, fpu: fpu.MC68881, cir: true}, {model: m68kemu.M68010, fpu: fpu.MC68881, cir: true}}
	fftNativeFPU  = []fftMachine{
		{model: m68kemu.M68020, fpu: fpu.MC68881, coprocessor: true},
		{model: m68kemu.M68030, fpu: fpu.MC68882, coprocessor: true},
	}
)

var fftVariants = []fftVariant{
	{
		name: "Q15", target: asm.Target68000, machines: fftWithoutFPU,
		elem: 4, tw: 4, butterfly: fftQ15Butterfly,
		put: func(buf []byte, v complex128) { putQ(buf, v, 16) },
		get: func(buf []byte, n int) complex128 {
			re, im := int16(binary.BigEndian.Uint16(buf)), int16(binary.BigEndian.Uint16(buf[2:]))
			return complex(float64(re), float64(im)) * complex(float64(n)/32768, 0)
		},
		minSNR: 45,
	},
	{
		name: "Q31", target: asm.Target{CPU: asm.CPU68020}, machines: fft020,
		elem: 8, tw: 8, butterfly: fftQ31Butterfly,
		put: func(buf []byte, v complex128) { putQ(buf, v, 32) },
		get: func(buf []byte, n int) complex128 {
			re, im := int32(binary.BigEndian.Uint32(buf)), int32(binary.BigEndian.Uint32(buf[4:]))
			return complex(float64(re), float64(im)) * complex(float64(n)/(1<<31), 0)
		},
		minSNR: 130,
	},
	{
		name: "SFP004", target: asm.Target68000, machines: fftSFP004,
		elem: 16, tw: 16, prologue: fmt.Sprintf("\tLEA     $%X,A3\n", fftCIROperand),
		butterfly: fpCIRCode(fftFloatButterfly),
		put:       putDouble,
		get:       getDouble,
		minSNR:    250,
	},
	{
		name: "FPU", target: asm.Target{CPU: asm.CPU68020, Features: asm.FeatFPU}, machines: fftNativeFPU,
		elem: 16, tw: 16, butterfly: fpLineCode(fftFloatButterfly),
		put:    putDouble,
		get:    getDouble,
		minSNR: 250,
	},
}

// source returns the variant's program for an n-point FFT.
func (v fftVariant) source(n int) string {
	var swap strings.Builder
	for off := 0; off < v.elem; off += 4 {
		fmt.Fprintf(&swap, "\tMOVE.L  %[1]d(A0,D0.L),D2\n\tMOVE.L  %[1]d(A0,D1.L),D3\n\tMOVE.L  D3,%[1]d(A0,D0.L)\n\tMOVE.L  D2,%[1]d(A0,D1.L)\n", off)
	}
	hex := func(x int) string { return "$" + strconv.FormatInt(int64(x), 16) }
	return strings.NewReplacer(
		"SWAP\n", swap.String(),
		"PROLOGUE\n", v.prologue,
		"BUTTERFLY\n", v.butterfly,
		"TWIDDLES", hex(fftTwBase),
		"DATA", hex(fftDataBase),
		"END", hex(fftDataBase+n*v.elem),
		"#N-2", "#"+strconv.Itoa(n-2),
		"#HALF", "#"+strconv.Itoa(n/2*v.elem),
		"#ELEM", "#"+strconv.Itoa(v.elem),
		"#STRIDE", "#"+strconv.Itoa(n/2*v.tw),
		"#TW", "#"+strconv.Itoa(v.tw),
	).Replace(fftSkeleton)
}

// fftInput is a deterministic test signal: two tones and some noise, with a
// magnitude below 0.8 so the fixed-point variants cannot overflow.
func fftInput(n int) []complex128 {
	rng := rand.New(rand.NewSource(1))
	x := make([]complex128, n)
	for i := range x {
		t := 2 * math.Pi * float64(i) / float64(n)
		re := 0.4*math.Cos(5*t) + 0.2*math.Sin(37*t) + 0.1*(rng.Float64()-0.5)
		im := 0.3*math.Sin(11*t) + 0.1*(rng.Float64()-0.5)
		x[i] = complex(re, im)
	}
	return x
}

// referenceFFT is a direct O(n²) DFT in float64.
func referenceFFT(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := range out {
		var sum complex128
		for j, v := range x {
			sum += v * cmplx.Rect(1, -2*math.Pi*float64(j*k%n)/float64(n))
		}
		out[k] = sum
	}
	return out
}

// fftRun is a CPU with a variant's program loaded.
type fftRun struct {
	cpu      m68kemu.CPU
	ram      *m68kemu.RAM
	v        fftVariant
	n        int
	input    []complex128 // the input as the variant stores it
	inputImg []byte
}

func newFFTRun(tb testing.TB, v fftVariant, m fftMachine, n int) *fftRun {
	tb.Helper()
	code, err := asm.AssembleStringWithOptions(v.source(n), asm.ParseOptions{Target: v.target})
	if err != nil {
		tb.Fatalf("assembler: %v", err)
	}
	ram := m68kemu.NewRAM(0, fftRAMSize)
	ram.Write(m68kemu.Long, 0, 0x1000)
	ram.Write(m68kemu.Long, 4, fftCodeBase)
	writeBytes(ram, fftCodeBase, code)

	tw := make([]byte, n/2*v.tw)
	for k := range n / 2 {
		v.put(tw[k*v.tw:], cmplx.Rect(1, -2*math.Pi*float64(k)/float64(n)))
	}
	writeBytes(ram, fftTwBase, tw)

	// Quantize the input to the variant's format so the reference sees the
	// same samples as the emulated FFT.
	img := make([]byte, n*v.elem)
	input := fftInput(n)
	for i, x := range input {
		v.put(img[i*v.elem:], x)
		input[i] = v.get(img[i*v.elem:], 1)
	}

	devices := []m68kemu.Device{ram}
	options := []m68kemu.Option{m68kemu.WithModel(m.model)}
	var cir *fpu.CIR
	switch {
	case m.cir:
		cir = fpu.NewCIR(fpu.New(m.fpu), fftCIRBase)
		devices = append(devices, cir)
	case m.coprocessor:
		options = append(options, m68kemu.WithCoprocessor(1, fpu.New(m.fpu).Coprocessor()))
	}
	cpu, err := m68kemu.NewCPU(m68kemu.NewBus(devices...), options...)
	if err != nil {
		tb.Fatal(err)
	}
	if cir != nil {
		cir.Clock = cpu.Cycles // the FPU runs at the CPU's clock
	}
	return &fftRun{cpu: cpu, ram: ram, v: v, n: n, input: input, inputImg: img}
}

func writeBytes(ram *m68kemu.RAM, base uint32, data []byte) {
	i := 0
	for ; i+4 <= len(data); i += 4 {
		ram.Write(m68kemu.Long, base+uint32(i), binary.BigEndian.Uint32(data[i:]))
	}
	for ; i < len(data); i++ {
		ram.Write(m68kemu.Byte, base+uint32(i), uint32(data[i]))
	}
}

// reset resets the CPU and restores the input samples.
func (r *fftRun) reset(tb testing.TB) {
	if err := r.cpu.Reset(); err != nil {
		tb.Fatalf("reset: %v", err)
	}
	writeBytes(r.ram, fftDataBase, r.inputImg)
}

// measure runs the FFT once to its halt loop and returns the instructions
// and cycles it took.
func (r *fftRun) measure(tb testing.TB) (instructions, cycles uint64) {
	tb.Helper()
	r.reset(tb)
	result, err := r.cpu.RunUntil(m68kemu.RunUntilOptions{MaxInstructions: 100_000_000, StopAtPC: []uint32{fftDonePC}, StopOnException: true})
	if err != nil {
		tb.Fatalf("run: %v", err)
	}
	if result.Reason != m68kemu.RunStopPC {
		tb.Fatalf("FFT stopped with %v at PC %08x, want the halt loop at %08x", result.Reason, result.PC, fftDonePC)
	}
	return result.Instructions, result.Cycles
}

// snr compares the FFT in RAM with the reference and returns the
// signal-to-noise ratio in dB.
func (r *fftRun) snr(tb testing.TB) float64 {
	tb.Helper()
	buf := make([]byte, r.n*r.v.elem)
	for i := 0; i < len(buf); i += 4 {
		v, err := r.ram.Read(m68kemu.Long, fftDataBase+uint32(i))
		if err != nil {
			tb.Fatal(err)
		}
		binary.BigEndian.PutUint32(buf[i:], v)
	}
	var signal, noise float64
	for k, want := range referenceFFT(r.input) {
		got := r.v.get(buf[k*r.v.elem:], r.n)
		signal += real(want)*real(want) + imag(want)*imag(want)
		d := got - want
		noise += real(d)*real(d) + imag(d)*imag(d)
	}
	if noise == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(signal/noise)
}

func TestFFTPrograms(t *testing.T) {
	for _, v := range fftVariants {
		for _, m := range v.machines {
			t.Run(v.name+"/"+m.String(), func(t *testing.T) {
				r := newFFTRun(t, v, m, 256)
				instructions, cycles := r.measure(t)
				snr := r.snr(t)
				if snr < v.minSNR {
					t.Fatalf("SNR %.1f dB, want at least %.0f dB", snr, v.minSNR)
				}
				t.Logf("%d instructions, %d cycles, SNR %.1f dB", instructions, cycles, snr)
			})
		}
	}
}

// BenchmarkFFT runs a 1024-point complex FFT for every variant and machine.
// Run it with
//
//	go test -run '^$' -bench BenchmarkFFT .
func BenchmarkFFT(b *testing.B) {
	for _, v := range fftVariants {
		for _, m := range v.machines {
			b.Run(v.name+"/"+m.String(), func(b *testing.B) {
				r := newFFTRun(b, v, m, fftBenchSize)
				instructions, cycles := r.measure(b)
				snr := r.snr(b)
				if snr < v.minSNR {
					b.Fatalf("SNR %.1f dB, want at least %.0f dB", snr, v.minSNR)
				}

				for b.Loop() {
					b.StopTimer()
					r.reset(b)
					b.StartTimer()
					if err := r.cpu.RunCycles(cycles); err != nil {
						b.Fatalf("RunCycles: %v", err)
					}
				}
				if pc := r.cpu.Registers().PC; pc != fftDonePC {
					b.Fatalf("FFT ended at PC %08x, want the halt loop at %08x", pc, fftDonePC)
				}

				b.ReportMetric(float64(instructions), "instrs/fft")
				b.ReportMetric(float64(cycles), "cycles/fft")
				b.ReportMetric(float64(instructions)*float64(b.N)/b.Elapsed().Seconds()/1e6, "Minstr/s")
				b.ReportMetric(snr, "SNR-dB")
			})
		}
	}
}
