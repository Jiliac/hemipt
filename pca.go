package main

import (
	"fmt"

	"math"
	"time"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

// Phase 1 (short): fitness function collect some traces and then initialize the
// dynamicPCA.
// Phase 2 (short): collect data to recenter.
// Phase 3 (short): collect data to rotate the basis.
// Phase 4 (indefinite): full DynPCA algorithm. rotate and add new axis.

const (
	pcaInitDim = 10

	// Phase 2
	phase2Dur = time.Second
	// Phase 3
	phase3Dur = phase2Dur
)

type dynamicPCA struct {
	// Y = (X-center)' * basis
	centers [mapSize]float64
	basis   *mat.Dense

	sampleN int
	sums    [mapSize]float64
	covMat  *mat.Dense

	startT, recenterT      time.Time
	phase2, phase3, phase4 bool
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
	vecs := new(mat.Dense)
	pc.VectorsTo(vecs)
	dynpca.basis = mat.DenseCopyOf(vecs.Slice(0, mapSize, 0, pcaInitDim))
	//
	dynpca.covMat = mat.NewDense(pcaInitDim, pcaInitDim, nil)
	vars := pc.VarsTo(nil)
	for i := 0; i < pcaInitDim; i++ {
		dynpca.covMat.Set(i, i, float64(dynpca.sampleN)*vars[i])
	}
	//
	dynpca.phase2 = true
	dynpca.startT = time.Now()

	return ok, dynpca
}

func (dynpca *dynamicPCA) newSample(trace []byte) {
	if dynpca.phase2 && time.Now().Sub(dynpca.startT) > phase2Dur {
		fmt.Println("PHASE 3")
		dynpca.recenter()
		dynpca.recenterT = time.Now()
		dynpca.phase2, dynpca.phase3 = false, true
		//
	} else if dynpca.phase3 && time.Now().Sub(dynpca.recenterT) > phase3Dur {
		fmt.Println("PHASE 4")
		// @TODO: rotate covmat and start adding new axis
		dynpca.phase3, dynpca.phase4 = false, true
	}

	// ** 1. Center data **
	dynpca.sampleN++
	sampMat := mat.NewDense(1, mapSize, nil)
	for i, tr := range trace {
		v := logVals[tr]
		dynpca.sums[i] += v
		sampMat.Set(0, i, v-dynpca.centers[i])
	}

	// ** 2. Project **
	projMat := new(mat.Dense)
	projMat.Mul(sampMat, dynpca.basis)

	// ** 3. Update covariance matrix **
	covs := new(mat.Dense)
	covs.Mul(projMat.T(), projMat)
	dynpca.covMat.Add(dynpca.covMat, covs)
}

func (dynpca *dynamicPCA) recenter() {
	var diff float64
	n := float64(dynpca.sampleN)
	newSampN := dynpca.sampleN / 10
	for i := 0; i < mapSize; i++ {
		c := dynpca.sums[i] / n
		d := dynpca.centers[i] - c
		diff += d * d
		//
		dynpca.centers[i] = c
		dynpca.sums[i] = c * float64(newSampN)
	}
	diff = math.Sqrt(diff)
	fmt.Printf("Centering difference: %.3v\n", diff)

	m := new(mat.Dense)
	m.Scale(float64(newSampN)/n, dynpca.covMat)
	dynpca.covMat = m
	dynpca.sampleN = newSampN
}

func (dynpca *dynamicPCA) String() (str string) {
	dynpca.recenter()

	str = fmt.Sprintf("#sample: %.3v\n", float64(dynpca.sampleN))
	//
	var m mat.Dense
	m.Scale(1/float64(dynpca.sampleN), dynpca.covMat)
	str += fmt.Sprintf("Covariance Matrix:\n%.3v", mat.Formatted(&m))
	return str
}
