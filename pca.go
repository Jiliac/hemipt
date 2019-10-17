package main

import (
	"fmt"
	"log"

	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

// Phase 1 (short): fitness function collect some traces and then initialize the
// dynamicPCA. (Implemented in newDynPCA).
// Phase 2 (short): collect data to recenter.
// Phase 3 (short): collect data to rotate the basis.
// Phase 4 (indefinite): full DynPCA algorithm. rotate and add new axis.

const (
	pcaInitDim = 10

	// Phase 2
	phase2Dur = time.Second
	// Phase 3
	phase3Dur     = phase2Dur
	convCritFloor = 0.05 // Floor to apply rotation.
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
		ok := dynpca.rotate()
		if ok {
			dynpca.phase3, dynpca.phase4 = false, true
		} else {
			dynpca.recenterT = time.Now()
		}
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

func (dynpca *dynamicPCA) rotate() (ok bool) {
	// ** 1. Prepare data **
	_, basisSize := dynpca.basis.Dims()
	fmt.Printf("basisSize = %+v\n", basisSize)
	covs := make([]float64, basisSize*basisSize)
	for i := 0; i < basisSize; i++ {
		for j := 0; j < basisSize; j++ {
			cov := dynpca.covMat.At(i, j) / float64(dynpca.sampleN)
			covs[i*basisSize+j] = cov
		}
	}
	covMat := mat.NewSymDense(basisSize, covs)

	// ** 2. Rotate **
	ok, eVals, eVecs := factorize(covMat, basisSize)
	if !ok {
		log.Print("Could not factorize covariance matrix.")
		return ok
	}
	ok = true

	// Test print
	convCrit := computeConvergence(eVecs)
	fmt.Printf("convCrit = %.3v\n", convCrit)
	fmt.Printf("eVals = %.3v\n", eVals)
	fmt.Printf("Eigen vectors:\n%.3v\n", mat.Formatted(eVecs))
	m := new(mat.Dense)
	m.Scale(1/float64(dynpca.sampleN), dynpca.covMat)
	m.Mul(eVecs.T(), m)
	m.Mul(m, eVecs)
	fmt.Printf("Rotated covariance matrix:\n%.3v\n", mat.Formatted(m))

	// ** 3. Apply decomposition **
	if convCrit > convCritFloor {
		dynpca.covMat = mat.NewDense(basisSize, basisSize, nil)
		for i := 0; i < basisSize; i++ {
			dynpca.covMat.Set(i, i, eVals[i]*float64(dynpca.sampleN))
		}
		dynpca.basis.Mul(dynpca.basis, eVecs)
	}

	return ok
}
func factorize(symMat *mat.SymDense, basisSize int) (
	ok bool, eVals []float64, eVecs *mat.Dense) {
	// Gonum library is a little weird about its eigen decomposiiton
	// implementation. So make a helper function around it.

	var eigsym mat.EigenSym
	ok = eigsym.Factorize(symMat, true)
	if !ok {
		return
	}
	//
	vars := eigsym.Values(nil)
	eVecs = new(mat.Dense)
	eigsym.VectorsTo(eVecs)

	var (
		perm    = make([]int, basisSize)
		permMat = new(mat.Dense)
	)
	eVals = make([]float64, basisSize)
	// a. Find permutation to order eigen vectors/values.
	for i := range perm {
		perm[i] = i
	}
	sort.Slice(perm, func(i, j int) bool {
		indexI, indexJ := perm[i], perm[j]
		return vars[indexI] > vars[indexJ]
	})
	// b. Apply permutation
	for i, index := range perm {
		eVals[i] = vars[index]
	}
	permMat.Permutation(basisSize, perm)
	eVecs.Mul(eVecs, permMat)

	return ok, eVals, eVecs
}
func computeConvergence(ev *mat.Dense) (convCrit float64) {
	r, c := ev.Dims()
	//
	for j := 0; j < c; j++ {
		var maxJ, v float64
		for i := 0; i < r; i++ {
			v = ev.At(i, j)
			v *= v
			if v > maxJ {
				maxJ = v
			}
			convCrit += v
		}
		convCrit -= maxJ
	}
	//
	convCrit /= float64(c)
	return convCrit
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
