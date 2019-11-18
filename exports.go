package main

import (
	"fmt"
	"log"

	"encoding/csv"
	"io/ioutil"
	"os"
	"path/filepath"
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

func exportHistos(statSlice []*basisStats, path string) {
	if len(statSlice) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	records := [][]string{[]string{
		"seed_n", "dim_n", "bin_n", "start", "end", "count",
	}}
	for i, stats := range statSlice {
		histos, steps := stats.histos, stats.steps
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
			fmt.Sprintf("%f", pca.sqNorm*normalizer),
		}
		for i := 0; i < basisSize; i++ {
			pcaEntry = append(pcaEntry, fmt.Sprintf("%f", m.At(i, i)))
		}
		records = append(records, pcaEntry)
	}

	writeCSV(w, records)
}

func exportHashes(seeds []*seedT, path string) {
	if len(seeds) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	records := [][]string{[]string{"index", "hash"}}
	for i, seed := range seeds {
		records = append(records, []string{
			fmt.Sprintf("%d", i), fmt.Sprintf("0x%x", seed.hash)})
	}

	writeCSV(w, records)
}

// ********************************************
// ***** Export all kind of seed distance *****

func exportDistances(glbProj globalProjection, path string) {
	if len(glbProj.cleanedSeeds) == 0 {
		return
	}
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	pcas, centMats, seedMats := glbProj.pcas, glbProj.centMats, glbProj.seedMats
	vars, centProjs, seedProjs := glbProj.vars, glbProj.centProjs, glbProj.seedProjs

	var wg sync.WaitGroup
	subRecs := make([][][]string, len(centMats))
	rrv := func(m *mat.Dense) []float64 { return m.RawRowView(0) }
	for i := range centMats {
		wg.Add(1)
		go func(i int) {

			i1 := fmt.Sprintf("%d", i)
			sqNorm := pcas[i].sqNorm / float64(pcas[i].sampleN)
			covMat, _, _ := inverseMat(pcas[i])
			det, _ := mat.LogDet(covMat)
			subRecs[i] = [][]string{
				[]string{i1, i1, "s2c_full_eucli", fmt.Sprintf("%f", euclideanDist(
					rrv(seedMats[i]), rrv(centMats[i])))},
				[]string{i1, i1, "s2c_proj_eucli", fmt.Sprintf("%f", euclideanDist(
					seedProjs[i].RawRowView(0), centProjs[i].RawRowView(0)))},
				[]string{i1, i1, "s2c_maha", fmt.Sprintf("%f", mahaDist(
					rrv(seedProjs[i]), rrv(centProjs[i]), vars))},
				[]string{i1, i1, "sq_norm", fmt.Sprintf("%f", sqNorm)},
				[]string{i1, i1, "log_det", fmt.Sprintf("%f", det)},
			}

			virtPoint := &dynamicPCA{
				sampleN: 1,
				covMat:  idMat(len(vars)),
				basis:   glbProj.basis,
				centers: pcas[i].centers,
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
				//
				if !logFreq {
					continue
				}
				oki, ffi := getPCAFF(glbProj.cleanedSeeds[i])
				okj, ffj := getPCAFF(glbProj.cleanedSeeds[j])
				if !oki || !okj {
					log.Println("Skip MLE divergence.")
					continue
				}
				subRecs[i] = append(subRecs[i], [][]string{
					[]string{i1, i2, "mle_divergence", fmt.Sprintf("%f",
						computeMLEDiv(ffi.hashesF, ffj.hashesF))},
					[]string{i2, i1, "mle_divergence", fmt.Sprintf("%f",
						computeMLEDiv(ffj.hashesF, ffi.hashesF))},
				}...)
			}

			for j := 0; j < len(centMats); j++ {
				i2 := fmt.Sprintf("%d", j)
				subRecs[i] = append(subRecs[i], []string{i1, i2, "virtual_div",
					fmt.Sprintf("%f", klDiv(pcas[j], virtPoint))})
				subRecs[i] = append(subRecs[i], []string{i1, i2, "virtual_div_rev",
					fmt.Sprintf("%f", klDiv(virtPoint, pcas[j]))})
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

	return
}
func idMat(n int) *mat.Dense {
	m := mat.NewDense(n, n, nil)
	for i := 0; i < n; i++ {
		m.Set(i, i, 1)
	}
	return m
}

func exportCoor(glbProj globalProjection, path string) {
	ok, w := makeCSVFile(path)
	if !ok {
		return
	}

	header := []string{"kind", "index"}
	for i := range glbProj.vars {
		header = append(header, fmt.Sprintf("pc%d", i))
	}
	records := [][]string{header}

	// a. Export variances
	varsStrs := []string{"variance", "0"}
	for _, v := range glbProj.vars {
		varsStrs = append(varsStrs, fmt.Sprintf("%f", v))
	}
	records = append(records, varsStrs)
	// b. Export centers' coordinates
	rrv := func(m *mat.Dense) []float64 { return m.RawRowView(0) }
	for i, c := range glbProj.centProjs {
		strs := []string{"center", fmt.Sprintf("%d", i)}
		for _, v := range rrv(c) {
			strs = append(strs, fmt.Sprintf("%f", v))
		}
		records = append(records, strs)
	}
	// c. Export seeds' coordinates
	for i, s := range glbProj.seedProjs {
		strs := []string{"seed", fmt.Sprintf("%d", i)}
		for _, v := range rrv(s) {
			strs = append(strs, fmt.Sprintf("%f", v))
		}
		records = append(records, strs)
	}

	writeCSV(w, records)
}

// *****************************************************************************
// *************************** Save Seeds on Disk ******************************

func saveSeeds(outDir string, seeds []*seedT) {
	dir := filepath.Join(outDir, "seeds")
	err := os.Mkdir(dir, 0755)
	if err != nil {
		log.Printf("Couldn't create seed directory: %v.\n", err)
		return
	}

	for i, seed := range seeds {
		in := seed.input
		if seed.hash == 0 {
			fmt.Printf("Seed %d has a nil hash? (len=%d)\n", i, len(in))
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("%x", seed.hash))
		err := ioutil.WriteFile(path, in, 0644)
		if err != nil {
			log.Printf("Couldn't write seed %d: %v.\n", i, err)
		}
	}
}
