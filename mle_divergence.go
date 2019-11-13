package main

import (
	"fmt"
	"log"
)

func testMLEDiv(seeds []*seedT) {
	if len(seeds) < 2 {
		log.Println("Not enough seeds to test MLE Divergence.")
		return
	}

	ok0, pcaFF0 := getPCAFF(seeds[0])
	ok1, pcaFF1 := getPCAFF(seeds[1])
	if !ok0 || !ok1 {
		log.Println("Couldn't get PCA fit func to test MLE Divergence.")
		return
	}

	div := computeMLEDiv(pcaFF0.hashesF, pcaFF1.hashesF)
	fmt.Printf("div = %.3v\n", div)
}

func computeMLEDiv(hashes1, hashes2 map[uint64]byte) (div float64) {
	return div
}
