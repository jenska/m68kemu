package m68kemu

type opcodeMeta struct {
	x        uint8
	y        uint8
	srcMode  uint8
	srcIndex uint8
	dstMode  uint8
	dstIndex uint8
	opSize   Size
}

var opcodeMetaTable = buildOpcodeMetaTable()

func buildOpcodeMetaTable() [0x10000]opcodeMeta {
	var table [0x10000]opcodeMeta
	for opcode := 0; opcode < len(table); opcode++ {
		ir := uint16(opcode)
		srcMode := (ir >> 3) & 0x7
		srcIndex := srcMode
		if srcMode == 7 {
			srcIndex += ir & 0x7
		}

		dstMode := (ir >> 6) & 0x7
		dstIndex := dstMode
		if dstMode == 7 {
			dstIndex += (ir >> 9) & 0x7
		}

		table[opcode] = opcodeMeta{
			x:        uint8((ir >> 9) & 0x7),
			y:        uint8(ir & 0x7),
			srcMode:  uint8(srcMode),
			srcIndex: uint8(srcIndex),
			dstMode:  uint8(dstMode),
			dstIndex: uint8(dstIndex),
			opSize:   opSizes[(ir>>6)&0x3],
		}
	}
	return table
}
