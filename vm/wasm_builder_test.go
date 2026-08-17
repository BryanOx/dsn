package vm

import "bytes"

// wasmBuilder constructs valid WASM binary modules for tests.
// It handles section sizing and LEB128 encoding automatically.
type wasmFuncBody struct {
	locals []byte // local declarations (count + types, or empty for 0 locals)
	body   []byte // function body instructions
}

type wasmBuilder struct {
	types    []wasmFuncType
	imports  []wasmImport
	funcs    []uint32         // type indices for module functions
	memPages uint32           // 0 = no memory
	exports  []wasmExport
	bodies   []*wasmFuncBody  // function bodies with local declarations
}

type wasmFuncType struct {
	params  []byte // wasm value types: 0x7f=i32, 0x7e=i64, 0x7d=f32, 0x7c=f64
	results []byte
}

type wasmImport struct {
	module string
	name   string
	kind   byte // 0x00 = func, 0x02 = mem
	// For func imports:
	funcTypeIdx uint32
	// For mem imports:
	memMin uint32
}

type wasmExport struct {
	name  string
	kind  byte // 0x00 = func, 0x02 = mem
	index uint32
}

func newWasmBuilder() *wasmBuilder {
	return &wasmBuilder{}
}

// addFuncType adds a function type and returns its index.
func (wb *wasmBuilder) addFuncType(params []byte, results []byte) uint32 {
	idx := uint32(len(wb.types))
	wb.types = append(wb.types, wasmFuncType{params: params, results: results})
	return idx
}

// addFuncImport adds a function import and returns the import's func index.
func (wb *wasmBuilder) addFuncImport(module, name string, typeIdx uint32) uint32 {
	idx := uint32(len(wb.imports))
	wb.imports = append(wb.imports, wasmImport{
		module:      module,
		name:        name,
		kind:        0x00,
		funcTypeIdx: typeIdx,
	})
	return idx
}

// addFunc adds a module function with the given type index, zero locals, and body instructions.
// Returns the global WASM function index (imports come first).
func (wb *wasmBuilder) addFunc(typeIdx uint32, body []byte) uint32 {
	return wb.addFuncWithLocals(typeIdx, nil, body)
}

// addFuncWithLocals adds a module function with explicit local declarations.
// locals is a flat list of wasm value types (e.g., []byte{0x7f, 0x7f} for two i32 locals).
// Returns the global WASM function index (imports come first, then module functions).
func (wb *wasmBuilder) addFuncWithLocals(typeIdx uint32, locals []byte, body []byte) uint32 {
	idx := uint32(len(wb.imports) + len(wb.funcs))
	wb.funcs = append(wb.funcs, typeIdx)
	// Encode locals as WASM local declarations: single group of N locals of same type
	var localDecl []byte
	if len(locals) > 0 {
		localDecl = append(localDecl, byte(len(locals)))
		localDecl = append(localDecl, locals[0]) // assume all same type for simplicity
	}
	wb.bodies = append(wb.bodies, &wasmFuncBody{locals: localDecl, body: body})
	return idx
}

// addMemory sets the minimum memory pages (0 = no memory section).
func (wb *wasmBuilder) addMemory(minPages uint32) {
	wb.memPages = minPages
}

// addExport adds an export entry.
func (wb *wasmBuilder) addExport(name string, kind byte, index uint32) {
	wb.exports = append(wb.exports, wasmExport{name: name, kind: kind, index: index})
}

// build produces a valid WASM binary module.
func (wb *wasmBuilder) build() []byte {
	var buf bytes.Buffer

	// Magic + version
	buf.Write([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})

	// Type section (id=1)
	if len(wb.types) > 0 {
		var sec bytes.Buffer
		encodeULEB128(&sec, uint32(len(wb.types)))
		for _, t := range wb.types {
			sec.WriteByte(0x60) // func type
			encodeULEB128(&sec, uint32(len(t.params)))
			sec.Write(t.params)
			encodeULEB128(&sec, uint32(len(t.results)))
			sec.Write(t.results)
		}
		writeSection(&buf, 0x01, sec.Bytes())
	}

	// Import section (id=2)
	if len(wb.imports) > 0 {
		var sec bytes.Buffer
		encodeULEB128(&sec, uint32(len(wb.imports)))
		for _, imp := range wb.imports {
			encodeString(&sec, imp.module)
			encodeString(&sec, imp.name)
			sec.WriteByte(imp.kind)
			if imp.kind == 0x00 { // func
				encodeULEB128(&sec, imp.funcTypeIdx)
			} else if imp.kind == 0x02 { // mem
				// limits: flag=0x00 (no max), min
				sec.WriteByte(0x00)
				encodeULEB128(&sec, imp.memMin)
			}
		}
		writeSection(&buf, 0x02, sec.Bytes())
	}

	// Function section (id=3) — maps func index to type index
	if len(wb.funcs) > 0 {
		var sec bytes.Buffer
		encodeULEB128(&sec, uint32(len(wb.funcs)))
		for _, typeIdx := range wb.funcs {
			encodeULEB128(&sec, typeIdx)
		}
		writeSection(&buf, 0x03, sec.Bytes())
	}

	// Memory section (id=5)
	if wb.memPages > 0 {
		var sec bytes.Buffer
		encodeULEB128(&sec, 1) // 1 memory
		sec.WriteByte(0x00)    // flag: no max
		encodeULEB128(&sec, wb.memPages)
		writeSection(&buf, 0x05, sec.Bytes())
	}

	// Export section (id=7)
	if len(wb.exports) > 0 {
		var sec bytes.Buffer
		encodeULEB128(&sec, uint32(len(wb.exports)))
		for _, exp := range wb.exports {
			encodeString(&sec, exp.name)
			sec.WriteByte(exp.kind)
			encodeULEB128(&sec, exp.index)
		}
		writeSection(&buf, 0x07, sec.Bytes())
	}

	// Code section (id=10)
	if len(wb.bodies) > 0 {
		var sec bytes.Buffer
		encodeULEB128(&sec, uint32(len(wb.bodies)))
		for _, fb := range wb.bodies {
			var bodyBuf bytes.Buffer
			// Local declarations
			if len(fb.locals) > 0 {
				bodyBuf.Write(fb.locals)
			} else {
				bodyBuf.WriteByte(0x00) // 0 locals
			}
			bodyBuf.Write(fb.body)
			// Ensure the body ends with the 0x0b (end) opcode
			if len(fb.body) == 0 || fb.body[len(fb.body)-1] != 0x0b {
				bodyBuf.WriteByte(0x0b)
			}
			// Body = locals + instructions
			encodeULEB128(&sec, uint32(bodyBuf.Len()))
			sec.Write(bodyBuf.Bytes())
		}
		writeSection(&buf, 0x0a, sec.Bytes())
	}

	return buf.Bytes()
}

func writeSection(buf *bytes.Buffer, id byte, content []byte) {
	buf.WriteByte(id)
	encodeULEB128(buf, uint32(len(content)))
	buf.Write(content)
}

func encodeULEB128(buf *bytes.Buffer, val uint32) {
	for {
		b := byte(val & 0x7f)
		val >>= 7
		if val != 0 {
			b |= 0x80
		}
		buf.WriteByte(b)
		if val == 0 {
			break
		}
	}
}

func encodeString(buf *bytes.Buffer, s string) {
	encodeULEB128(buf, uint32(len(s)))
	buf.WriteString(s)
}

// encodeSignedLEB128Bytes encodes an int32 as signed LEB128 and returns raw bytes.
func encodeSignedLEB128Bytes(val int32) []byte {
	var result []byte
	for {
		b := byte(val & 0x7f)
		val >>= 7
		if (val == 0 && b&0x40 == 0) || (val == -1 && b&0x40 != 0) {
			result = append(result, b)
			break
		}
		b |= 0x80
		result = append(result, b)
	}
	return result
}

// encodeULEB128Bytes encodes a uint32 as unsigned LEB128 and returns raw bytes.
func encodeULEB128Bytes(val uint32) []byte {
	var result []byte
	for {
		b := byte(val & 0x7f)
		val >>= 7
		if val != 0 {
			b |= 0x80
		}
		result = append(result, b)
		if val == 0 {
			break
		}
	}
	return result
}
