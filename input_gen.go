package main

type inputGen interface {
	generate() (testCase []byte)
}

var (
	_ inputGen = seedCopier([]byte{})
)

// *****************************************************************************
// ******************************** Seed Gen ***********************************
// Just a dummy input generator: always return the seed as is.

type seedCopier []byte

func (sc seedCopier) generate() []byte { return []byte(sc) }
