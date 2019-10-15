package main

import (
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

type dynamicPCA struct {
	// Y = (X-center)' * basis
	centers [mapSize]float64
	basis   *mat.Dense

	sampleN int
	sums    [mapSize]float64
}

func newDynPCA(queue [][]byte) (ok bool, dynpca *dynamicPCA) {
	dynpca = new(dynamicPCA)

	// ** 1. Compute centers **
	for _, trace := range queue {
		for j, tr := range trace {
			dynpca.sums[j] += logVals[tr]
		}
	}

	// ** 2. Format Data**
	dynpca.sampleN = len(queue)
	samplesMat := mat.NewDense(dynpca.sampleN, mapSize, nil)
	for j := 0; j < mapSize; j++ {
		dynpca.centers[j] = dynpca.sums[j] / float64(dynpca.sampleN)
		for i := 0; i < dynpca.sampleN; i++ {
			y := logVals[queue[i][j]]
			samplesMat.Set(i, j, y-dynpca.centers[j])
		}
	}

	// ** 3. PCA **
	var pc stat.PC
	ok = pc.PrincipalComponents(samplesMat, nil)
	if !ok {
		return ok, dynpca
	}
	ok = true

	// ** 4. Prepare Structure **
	dynpca.basis = new(mat.Dense)
	pc.VectorsTo(dynpca.basis)

	return ok, dynpca
}
