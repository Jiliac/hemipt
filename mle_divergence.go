package main

import (
	"fmt"
	"log"

	"math"
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

	div1 := computeMLEDiv(pcaFF0.hashesF, pcaFF1.hashesF)
	div2 := computeMLEDiv(pcaFF1.hashesF, pcaFF0.hashesF)
	fmt.Printf("div1, div2: %.3v, %.3v\n", div1, div2)
}

func computeMLEDiv(hashesP, hashesQ map[uint64]byte) (div float64) {
	// ** Prepare data **
	// This data could be kept live, but at a high cost. Since this is just for
	// some experiments, rather keep this as a fixed cost on post-processing.
	var frequences [0x100][0x100]int
	var totP, totQ int
	for hash, fp := range hashesP {
		if fq, ok := hashesQ[hash]; ok {
			totP += int(fp)
			frequences[fp][fq]++
		} else {
			// @TODO: how to handle zeros??
			//totP += int(fp)
			//frequences[fp][0]++
		}
	}
	for _, fq := range hashesQ {
		// In the frequences table there isn't the couple where fp=0, fq>0,
		// because 0.log(0) = 0.
		totQ += int(fq)
	}

	// ** Compute divergence from data **
	for fp, freqI := range frequences {
		for fq, n := range freqI {
			if n == 0 {
				continue
			}
			div += float64(n*fp) * math.Log(float64(fp)/float64(fq))
		}
	}
	div /= float64(totP)
	div += math.Log(float64(totQ) / float64(totP))

	return div
}
