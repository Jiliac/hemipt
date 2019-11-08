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

func getPCA(seed *seedT) (ok bool, pca *dynamicPCA) {
	if seed == nil || seed.exec == nil {
		return
	}
	//
	df := seed.exec.discoveryFit
	var pcaFF *pcaFitFunc
	if ff, okConv := df.(fitnessMultiplexer); okConv {
		for _, ffi := range ff {
			if pcaFit, okConv := ffi.(*pcaFitFunc); okConv {
				pcaFF = pcaFit
			}
		}
	} else if pcaFit, okConv := df.(*pcaFitFunc); okConv {
		pcaFF = pcaFit
	}
	if pcaFF != nil && pcaFF.dynpca != nil && pcaFF.dynpca.phase4 {
		ok = true
		pca = pcaFF.dynpca
	}
	return ok, pca
}

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
		log.Println("No PCA found?")
		return false, globalProjection{}
	}
	fmt.Printf("len(seeds), len(pcas): %d, %d\n", len(seeds), len(pcas))

	mb := doMergeBasis(pcas)
	if mb.basis == nil { // Means there was an error.
		log.Println("Problem computing the global basis.")
		return false, globalProjection{}
	}

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
