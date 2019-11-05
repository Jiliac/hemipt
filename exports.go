package main

import (
	"fmt"
	"log"

	"encoding/csv"
	"os"
	"sync"

	"gonum.org/v1/gonum/mat"
)

// Helpers
func makeCSVFile(path string) (ok bool, w *csv.Writer) {
	f, err := os.Create(path)
	if err != nil {
		log.Printf("Problem opening histo CSV file: %v.\n", err)
		return
	}
	//
	ok, w = true, csv.NewWriter(f)
	return ok, w
}
func writeCSV(w *csv.Writer, records [][]string) {
	w.WriteAll(records)
	if err := w.Error(); err != nil {
		log.Printf("Couldn't record CSV: %v.\n", err)
	}
	// @TODO: Close the underlying file?
}

func exportHistos(pcas []*dynamicPCA, path string) {
	if len(pcas) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	records := [][]string{[]string{
		"seed_n", "dim_n", "bin_n", "start", "end", "count",
	}}
	for i := range pcas {
		histos, steps := pcas[i].stats.histos, pcas[i].stats.steps
		for j, histo := range histos {
			for k, cnt := range histo {
				records = append(records, []string{
					fmt.Sprintf("%d", i),                     // seed_n
					fmt.Sprintf("%d", j),                     // dim_n
					fmt.Sprintf("%d", k),                     // bin_n
					fmt.Sprintf("%f", float64(k)*steps[j]),   // start
					fmt.Sprintf("%f", float64(k+1)*steps[j]), // end
					fmt.Sprintf("%f", cnt),                   // count
				})
			}
		}
	}

	writeCSV(w, records)
}

func exportProjResults(pcas []*dynamicPCA, path string) {
	if len(pcas) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	header := []string{"sample_n", "proj_var", "tot_var"}
	for i := 0; i < pcaInitDim; i++ {
		header = append(header, fmt.Sprintf("pc%d_var", i))
	}
	records := [][]string{header}

	for _, pca := range pcas {
		var m mat.Dense
		var totSpaceVar float64
		normalizer := 1 / float64(pca.sampleN)
		_, basisSize := pca.basis.Dims()
		m.Scale(normalizer, pca.covMat)
		for i := 0; i < basisSize; i++ {
			totSpaceVar += m.At(i, i)
		}
		pcaEntry := []string{
			fmt.Sprintf("%d", pca.sampleN),
			fmt.Sprintf("%f", totSpaceVar),
			fmt.Sprintf("%f", pca.stats.sqNorm*normalizer),
		}
		for i := 0; i < basisSize; i++ {
			pcaEntry = append(pcaEntry, fmt.Sprintf("%f", m.At(i, i)))
		}
		records = append(records, pcaEntry)
	}

	writeCSV(w, records)
}

// ********************************************
// ***** Export all kind of seed distance *****

func exportDistances(seeds []*seedT, path string) {
	if len(seeds) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	// ** 1. Prepare trace and project them **
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
		return
	}
	centers, vars, glbBasis := mergeBasis(pcas)
	if glbBasis == nil { // Means there was an error.
		log.Println("Problem computing the global basis.")
		return
	}
	for i, pca := range pcas {
		c, s := mat.NewDense(1, mapSize, nil), mat.NewDense(1, mapSize, nil)
		seed := cleanedSeeds[i]
		for j, tr := range seed.trace {
			c.Set(0, j, pca.centers[j]-centers[j])
			s.Set(0, j, logVals[tr]-centers[j])
		}
		centMats, seedMats = append(centMats, c), append(seedMats, s)
		//
		cProj, sProj := new(mat.Dense), new(mat.Dense)
		cProj.Mul(c, glbBasis)
		sProj.Mul(s, glbBasis)
		centProjs, seedProjs = append(centProjs, cProj), append(seedProjs, sProj)
	}

	// ** 2. Compute distances and record them **
	var wg sync.WaitGroup
	subRecs := make([][][]string, len(centMats))
	rrv := func(m *mat.Dense) []float64 { return m.RawRowView(0) }
	for i := range centMats {
		wg.Add(1)
		go func(i int) {

			i1 := fmt.Sprintf("%d", i)
			subRecs[i] = [][]string{
				[]string{i1, i1, "s2c_full_eucli", fmt.Sprintf("%f", euclideanDist(
					rrv(seedMats[i]), rrv(centMats[i])))},
				[]string{i1, i1, "s2c_proj_eucli", fmt.Sprintf("%f", euclideanDist(
					seedProjs[i].RawRowView(0), centProjs[i].RawRowView(0)))},
				[]string{i1, i1, "s2c_maha", fmt.Sprintf("%f", mahaDist(
					rrv(seedProjs[i]), rrv(centProjs[i]), vars))},
			}
			for j := i + 1; j < len(centMats); j++ {
				i2 := fmt.Sprintf("%d", j)
				subRecs[i] = append(subRecs[i], [][]string{
					[]string{i1, i2, "c2c_full_eucli", fmt.Sprintf("%f", euclideanDist(
						rrv(centMats[i]), rrv(centMats[j])))},
					[]string{i1, i2, "c2c_proj_eucli", fmt.Sprintf("%f", euclideanDist(
						rrv(centProjs[i]), rrv(centProjs[j])))},
					[]string{i1, i2, "c2c_maha", fmt.Sprintf("%f", mahaDist(
						rrv(centProjs[i]), rrv(centProjs[j]), vars))},
					[]string{i1, i2, "s2s_full_eucli", fmt.Sprintf("%f", euclideanDist(
						rrv(seedMats[i]), rrv(seedMats[j])))},
					[]string{i1, i2, "s2s_proj_eucli", fmt.Sprintf("%f", euclideanDist(
						rrv(seedProjs[i]), rrv(seedProjs[j])))},
					[]string{i1, i2, "s2s_maha", fmt.Sprintf("%f", mahaDist(
						rrv(seedProjs[i]), rrv(seedProjs[j]), vars))},
					[]string{i1, i2, "divergence", fmt.Sprintf("%f",
						klDiv(pcas[i], pcas[j]))},
					[]string{i2, i1, "divergence", fmt.Sprintf("%f",
						klDiv(pcas[j], pcas[i]))},
				}...)
			}

			wg.Done()
		}(i)
	}

	wg.Wait()
	records := [][]string{[]string{"index1", "index2", "kind", "value"}}
	for _, subRec := range subRecs {
		records = append(records, subRec...)
	}
	writeCSV(w, records)
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
