package wgl

import (
	"reflect"
	"runtime"
	"syscall/js"
	"unsafe"
)

// GLTypes provides WebGL bindings.
type GLTypes struct {
	StaticDraw         js.Value
	ArrayBuffer        js.Value
	ElementArrayBuffer js.Value
	VertexShader       js.Value
	FragmentShader     js.Value
	Float              js.Value
	DepthTest          js.Value
	ColorBufferBit     js.Value
	DepthBufferBit     js.Value
	Triangles          js.Value
	UnsignedShort      js.Value
	LEqual             js.Value
	LineLoop           js.Value
	Line               js.Value
	LineStrip          js.Value
	DynamicDraw        js.Value
	CompileStatus      js.Value
	LinkStatus         js.Value
	Texture2D          js.Value
	TextureMinFilter   js.Value
	TextureMagFilter   js.Value
	TextureWrapS       js.Value
	TextureWrapT       js.Value
	ClampToEdge        js.Value
	Linear             js.Value
	RGBA               js.Value
	UnsignedByte       js.Value
	TriangleStrip      js.Value
}

func (types *GLTypes) New(gl js.Value) {
	types.StaticDraw = gl.Get("STATIC_DRAW")
	types.ArrayBuffer = gl.Get("ARRAY_BUFFER")
	types.ElementArrayBuffer = gl.Get("ELEMENT_ARRAY_BUFFER")
	types.VertexShader = gl.Get("VERTEX_SHADER")
	types.FragmentShader = gl.Get("FRAGMENT_SHADER")
	types.Float = gl.Get("FLOAT")
	types.DepthTest = gl.Get("DEPTH_TEST")
	types.ColorBufferBit = gl.Get("COLOR_BUFFER_BIT")
	types.DepthBufferBit = gl.Get("DEPTH_BUFFER_BIT")
	types.Triangles = gl.Get("TRIANGLES")
	types.UnsignedShort = gl.Get("UNSIGNED_SHORT")
	types.LEqual = gl.Get("LEQUAL")
	types.LineLoop = gl.Get("LINE_LOOP")
	types.Line = gl.Get("LINE")
	types.LineStrip = gl.Get("LINE_STRIP")
	types.DynamicDraw = gl.Get("DYNAMIC_DRAW")
	types.CompileStatus = gl.Get("COMPILE_STATUS")
	types.LinkStatus = gl.Get("LINK_STATUS")
	types.Texture2D = gl.Get("TEXTURE_2D")
	types.TextureMinFilter = gl.Get("TEXTURE_MIN_FILTER")
	types.TextureMagFilter = gl.Get("TEXTURE_MAG_FILTER")
	types.TextureWrapS = gl.Get("TEXTURE_WRAP_S")
	types.TextureWrapT = gl.Get("TEXTURE_WRAP_T")
	types.ClampToEdge = gl.Get("CLAMP_TO_EDGE")
	types.Linear = gl.Get("LINEAR")
	types.RGBA = gl.Get("RGBA")
	types.UnsignedByte = gl.Get("UNSIGNED_BYTE")
	types.TriangleStrip = gl.Get("TRIANGLE_STRIP")
}

func SliceToByteSlice(s interface{}) []byte {
	switch s := s.(type) {
	case []int8:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		return *(*[]byte)(unsafe.Pointer(h))
	case []int16:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 2
		h.Cap *= 2
		return *(*[]byte)(unsafe.Pointer(h))
	case []int32:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 4
		h.Cap *= 4
		return *(*[]byte)(unsafe.Pointer(h))
	case []int64:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 8
		h.Cap *= 8
		return *(*[]byte)(unsafe.Pointer(h))
	case []uint8:
		return s
	case []uint16:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 2
		h.Cap *= 2
		return *(*[]byte)(unsafe.Pointer(h))
	case []uint32:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 4
		h.Cap *= 4
		return *(*[]byte)(unsafe.Pointer(h))
	case []uint64:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 8
		h.Cap *= 8
		return *(*[]byte)(unsafe.Pointer(h))
	case []float32:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 4
		h.Cap *= 4
		return *(*[]byte)(unsafe.Pointer(h))
	case []float64:
		h := (*reflect.SliceHeader)(unsafe.Pointer(&s))
		h.Len *= 8
		h.Cap *= 8
		return *(*[]byte)(unsafe.Pointer(h))
	case js.Value: // Add case for JavaScript Value
		if s.InstanceOf(js.Global().Get("Uint8Array")) {
			buf := s.Get("buffer")
			typedArray := js.Global().Get("Uint8Array").New(buf)
			byteArray := make([]byte, typedArray.Get("byteLength").Int())
			js.CopyBytesToGo(byteArray, typedArray)
			return byteArray
		}
		panic("jsutil: unexpected value at sliceToBytesSlice: " + s.Type().String())
	default:
		panic("jsutil: unexpected value at sliceToBytesSlice: " + js.ValueOf(s).Type().String())
	}
}

func SliceToTypedArray(s interface{}) js.Value {
	switch s := s.(type) {
	case []int8:
		a := js.Global().Get("Uint8Array").New(len(s))
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int8Array").New(buf, a.Get("byteOffset"), a.Get("byteLength"))
	case []int16:
		a := js.Global().Get("Uint8Array").New(len(s) * 2)
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int16Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/2)
	case []int32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Int32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []uint8:
		a := js.Global().Get("Uint8Array").New(len(s))
		js.CopyBytesToJS(a, s)
		runtime.KeepAlive(s)
		return a
	case []uint16:
		a := js.Global().Get("Uint8Array").New(len(s) * 2)
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Uint16Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/2)
	case []uint32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Uint32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []float32:
		a := js.Global().Get("Uint8Array").New(len(s) * 4)
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Float32Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/4)
	case []float64:
		a := js.Global().Get("Uint8Array").New(len(s) * 8)
		js.CopyBytesToJS(a, SliceToByteSlice(s))
		runtime.KeepAlive(s)
		buf := a.Get("buffer")
		return js.Global().Get("Float64Array").New(buf, a.Get("byteOffset"), a.Get("byteLength").Int()/8)
	default:
		panic("jsutil: unexpected value at SliceToTypedArray: " + js.ValueOf(s).Type().String())
	}
}
