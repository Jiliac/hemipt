package main

import (
	"fmt"
	"log"

	"syscall"

	"gonum.org/v1/gonum/mat"
)

type runT struct {
	input []byte

	sig     syscall.Signal
	status  syscall.WaitStatus
	crashed bool
	hanged  bool

	trace []byte // Only used if is fit.
	hash  uint64
}

type seedT struct {
	runT

	execN   int
	running bool

	exec *executor
}

type globalProjection struct {
	pcas         []*dynamicPCA
	cleanedSeeds []*seedT

	mergedBasis

	centMats, seedMats   []*mat.Dense
	centProjs, seedProjs []*mat.Dense
}

// *****************************************************************************
// *************************** Interface Conversion ****************************

func getPCAFF(seed *seedT) (ok bool, pcaFF *pcaFitFunc) {
	if seed == nil || seed.exec == nil {
		return
	}
	//
	df := seed.exec.discoveryFit
	if ff, okConv := df.(fitnessMultiplexer); okConv {
		for _, ffi := range ff {
			if pcaFit, okConv := ffi.(*pcaFitFunc); okConv {
				ok, pcaFF = true, pcaFit
			}
		}
	} else if pcaFit, okConv := df.(*pcaFitFunc); okConv {
		ok, pcaFF = true, pcaFit
	}
	return ok, pcaFF
}
func getPCA(seed *seedT) (ok bool, pca *dynamicPCA) {
	if seed == nil || seed.exec == nil {
		return
	}
	//
	ok1, pcaFF := getPCAFF(seed)
	if ok1 && pcaFF != nil && pcaFF.dynpca != nil && pcaFF.dynpca.phase4 {
		ok = true
		pca = pcaFF.dynpca
	}
	return ok, pca
}
func getDivFF(seed *seedT) (ok bool, df *divFitness) {
	if seed == nil || seed.exec == nil {
		return
	}
	//
	dfit := seed.exec.discoveryFit
	if ff, okConv := dfit.(fitnessMultiplexer); okConv {
		for _, ffi := range ff {
			if fit, okConv := ffi.(*divFitness); okConv {
				ok, df = true, fit
			}
		}
	} else if fit, okConv := dfit.(*divFitness); okConv {
		ok, df = true, fit
	}
	return ok, df
}

// *****************************************************************************
// ****************************** Global Basis *********************************

func doGlbProjection(seeds []*seedT) (bool, globalProjection) {
	var (
		pcas                 []*dynamicPCA
		cleanedSeeds         []*seedT
		centMats, seedMats   []*mat.Dense
		centProjs, seedProjs []*mat.Dense
	)

	for _, seed := range seeds {
		ok, pca := getPCA(seed)
		if ok {
			pcas = append(pcas, pca)
			cleanedSeeds = append(cleanedSeeds, seed)
		}
	}
	if len(pcas) == 0 {
		log.Println("No seed has a PCA basis??")
		return false, globalProjection{}
	}
	fmt.Printf("len(seeds), len(pcas): %d, %d\n", len(seeds), len(pcas))

	basisSlice := prepareMerging(pcas)
	ok, mb := doMergeBasisBis(basisSlice, 2*pcaInitDim)
	if !ok { // There was an error.
		log.Println("Problem computing the global basis.")
		return false, globalProjection{}
	}
	varLossEval(basisSlice, mb) // Verbose

	for i, pca := range pcas {
		c, s := mat.NewDense(1, mapSize, nil), mat.NewDense(1, mapSize, nil)
		seed := cleanedSeeds[i]
		for j, tr := range seed.trace {
			c.Set(0, j, pca.centers[j]-mb.centers[j])
			s.Set(0, j, logVals[tr]-mb.centers[j])
		}
		centMats, seedMats = append(centMats, c), append(seedMats, s)
		//
		cProj, sProj := new(mat.Dense), new(mat.Dense)
		cProj.Mul(c, mb.basis)
		sProj.Mul(s, mb.basis)
		centProjs, seedProjs = append(centProjs, cProj), append(seedProjs, sProj)
	}

	return true, globalProjection{
		pcas:         pcas,
		cleanedSeeds: cleanedSeeds,
		mergedBasis:  mb,
		centMats:     centMats,
		seedMats:     seedMats,
		centProjs:    centProjs,
		seedProjs:    seedProjs,
	}
}

func varLossEval(basisSlice []mergedBasis, mb mergedBasis) {
	vars := make([]float64, mb.dimN)
	loss := newVarEval(basisSlice, mb.basis, vars)
	fmt.Printf("Overall projection loss: %.1f%%\n", 100*loss)
}

// *****************************************************************************
// ******************** Histo-based divergence estimation **********************

func checkHistos(seeds []*seedT) {
	fmt.Println("Checking histograms.")

	var statSlice []*basisStats
	for _, seed := range seeds {
		ok, df := getDivFF(seed)
		if ok {
			statSlice = append(statSlice, df.stats)
		}
	}
	if len(statSlice) == 0 {
		log.Println("Found no divergence fitness function.")
		return
	}
	fmt.Printf("len(seeds), #stats = %d, %d\n", len(seeds), len(statSlice))

	if len(statSlice) < 2 {
		return
	}
	div1 := klDivHisto(statSlice[0], statSlice[1])
	div2 := klDivHisto(statSlice[1], statSlice[0])
	fmt.Printf("div1, div2: %.3v, %.3v\n", div1, div2)
}
