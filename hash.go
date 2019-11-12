package main

import (
	"math"
	"reflect"
	"unsafe"
)

// *****************************************************************************
// ********************************** Hash *************************************
// From AFL.
// Is this MurmurHash3?

func rol(x uint64, shift uint) uint64 {
	return ((x << shift) | (x >> (64 - shift)))
}

func hashTrBits(traceBits []byte) (hash uint64) {
	const (
		hashSeed      = 0xa5b35705 // Nothing to do with fuzzing seeds...
		mapSize64 int = mapSize >> 3

		loopMult1  uint64 = 0x87c37b91114253d5
		loopMult2  uint64 = 0x4cf5ad432745937f
		loopMult3         = 5
		loopAdd           = 0x52dce729
		loopShift1        = 31
		loopShift2        = 27

		endMult1   uint64 = 0xff51afd7ed558ccd
		endMult2   uint64 = 0xc4ceb9fe1a85ec53
		endShift          = 33
		uint64Size        = 8
	)

	//data := (*[mapSize64]uint64)(unsafe.Pointer(traceBitPt))
	// Unsafe but fast conversion. @TODO: maybe we could do that only once.
	header := *(*reflect.SliceHeader)(unsafe.Pointer(&traceBits))
	header.Len /= uint64Size
	header.Cap /= uint64Size
	data := *(*[]uint64)(unsafe.Pointer(&header))

	hash = hashSeed ^ mapSize // ??

	for i := range data {
		k := data[i]
		k *= loopMult1
		k = rol(k, loopShift1)
		k *= loopMult2

		hash ^= k
		hash = rol(hash, loopShift2)
		hash = hash*5 + loopAdd
	}

	hash ^= hash >> endShift
	hash *= endMult1
	hash ^= hash >> endShift
	hash *= endMult2
	hash ^= hash >> endShift

	return hash
}

// *****************************************************************************
// ***************************** Trace value lookup ****************************

var logVals [0x100]float64

func init() {
	logReg := math.Log(regulizer)
	for i := 0; i < 0x100; i++ {
		logVals[i] = math.Log(float64(i)+regulizer) - logReg
	}
}
