package main

type inputGen interface {
	generate() (testCase []byte)
}
