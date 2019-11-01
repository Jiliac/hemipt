package main

import (
	"fmt"
	"log"

	"encoding/csv"
	"os"

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
