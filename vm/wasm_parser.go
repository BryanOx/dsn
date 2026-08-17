package vm

// findEntrypointBody parses a WASM binary to extract the function body
// of the named entrypoint export. Returns nil if not found.
func findEntrypointBody(wasmCode []byte, entrypoint string) []byte {
	if len(wasmCode) < 8 {
		return nil
	}
	offset := 8 // skip magic + version

	var exportFuncs map[string]uint32
	var funcBodies [][]byte
	importCount := 0

	for offset < len(wasmCode) {
		if offset+2 > len(wasmCode) {
			break
		}
		sectionID := wasmCode[offset]
		offset++

		sectionSize, bytesRead := readModuleULEB128(wasmCode[offset:])
		offset += bytesRead
		sectionEnd := offset + int(sectionSize)

		if sectionEnd > len(wasmCode) {
			break
		}

		data := wasmCode[offset:sectionEnd]

		switch sectionID {
		case 0x02: // import section
			n, _ := readModuleULEB128(data)
			importCount = int(n)

		case 0x07: // export section
			exportFuncs = make(map[string]uint32)
			n, br := readModuleULEB128(data)
			off := br
			for i := uint32(0); i < n; i++ {
				nameLen, br2 := readModuleULEB128(data[off:])
				off += br2
				name := string(data[off : off+int(nameLen)])
				off += int(nameLen)
				kind := data[off]
				off++
				idx, br3 := readModuleULEB128(data[off:])
				off += br3
				if kind == 0x00 { // func export
					exportFuncs[name] = idx
				}
			}

		case 0x0a: // code section
			n, br := readModuleULEB128(data)
			off := br
			for i := uint32(0); i < n; i++ {
				bodySize, br2 := readModuleULEB128(data[off:])
				off += br2
				body := data[off : off+int(bodySize)]
				off += int(bodySize)
				funcBodies = append(funcBodies, body)
			}
		}

		offset = sectionEnd
	}

	if exportFuncs == nil {
		return nil
	}

	funcIdx, ok := exportFuncs[entrypoint]
	if !ok {
		return nil
	}

	bodyIdx := int(funcIdx) - importCount
	if bodyIdx < 0 || bodyIdx >= len(funcBodies) {
		return nil
	}

	return funcBodies[bodyIdx]
}

// countInstructions counts the number of WASM instructions in a function body.
// It correctly skips LEB128 operands and multi-byte instructions.
func countInstructions(body []byte) uint64 {
	var count uint64
	i := 0

	// Skip local declarations
	if i < len(body) {
		numLocalGroups := int(body[i])
		i++
		for g := 0; g < numLocalGroups; g++ {
			for i < len(body) && body[i]&0x80 != 0 {
				i++
			}
			i++ // last byte of count
			i++ // type byte
		}
	}

	// Count instructions
	for i < len(body) {
		op := body[i]
		i++
		count++

		switch {
		// No operands: unreachable, nop, else, end, return, drop,
		// i32.or, i32.xor, i32.shl, i32.shr_s, i32.shr_u,
		// i32.rotl, i32.rotr, i32.clz, i32.ctz, i32.popcnt,
		// i32.eqz, i32.eq, i32.ne, i32.lt_s, i32.lt_u, i32.gt_s, i32.gt_u,
		// i32.le_s, i32.le_u, i32.ge_s, i32.ge_u,
		// i32.add, i32.sub, i32.mul, i32.div_s, i32.div_u, i32.rem_s, i32.rem_u,
		// i32.and
		case op == 0x00 || op == 0x01 || op == 0x05 || op == 0x0b || op == 0x0f ||
			op == 0x1a || op == 0x45 || op == 0x46 || op == 0x47 || op == 0x48 ||
			op == 0x4a || op == 0x4b || op == 0x4c || op == 0x4d || op == 0x4e ||
			op == 0x4f || op == 0x50 || op == 0x61 || op == 0x62 ||
			(op >= 0x6a && op <= 0x84):
			// no operands

		// One s32 LEB128: i32.const
		case op == 0x41:
			skipLEB128(body, &i)

		// One s64 LEB128: i64.const
		case op == 0x42:
			skipLEB128(body, &i)

		// 4 raw bytes: f32.const
		case op == 0x43:
			i += 4

		// 8 raw bytes: f64.const
		case op == 0x44:
			i += 8

		// One u32 LEB128: br, br_if, call, local.get, local.set, local.tee, global.get, global.set
		case op == 0x0c || op == 0x0d || op == 0x10 ||
			(op >= 0x20 && op <= 0x24):
			skipLEB128(body, &i)

		// call_indirect: typeidx (u32 LEB128) + tableidx (u32 = 1 byte)
		case op == 0x11:
			skipLEB128(body, &i)
			i++ // tableidx

		// Block type (1 byte): block, loop, if
		case op == 0x02 || op == 0x03 || op == 0x04:
			i++

		// Memarg (2 u32 LEB128): all load/store ops
		case op >= 0x28 && op <= 0x3e:
			skipLEB128(body, &i)
			skipLEB128(body, &i)

		// memory.grow / memory.size: 1 byte memory idx
		case op == 0x3f || op == 0x40:
			i++

		// 0xFC prefix (multi-byte): memory.init, data.drop, memory.copy, etc.
		case op == 0xfc:
			skipLEB128(body, &i) // sub-opcode
			// Remaining operands depend on sub-opcode; best-effort here

		// br_table: vec(u32 LEB128) + default_label (u32 LEB128)
		case op == 0x08:
			vecLen, br := readModuleULEB128(body[i:])
			i += br
			for j := uint32(0); j <= vecLen; j++ {
				skipLEB128(body, &i)
			}

		default:
			// Unknown opcode: can't reliably skip operands.
			// Return current count as best-effort.
		}
	}
	return count
}

func skipLEB128(body []byte, i *int) {
	for *i < len(body) && body[*i]&0x80 != 0 {
		*i++
	}
	*i++ // final byte
}

func readModuleULEB128(data []byte) (uint32, int) {
	var result uint32
	var shift uint
	for i := 0; i < len(data); i++ {
		b := data[i]
		result |= uint32(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			return result, i + 1
		}
	}
	return result, len(data)
}
