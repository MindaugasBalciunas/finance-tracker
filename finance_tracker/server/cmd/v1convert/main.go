// v1convert builds a v2 database from a v1 database or finances.json — the
// same conversion the server runs on first boot, as a standalone tool.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"

	"ft/internal/db"
	"ft/internal/importv1"
)

func main() {
	from := flag.String("from", "", "v1 finance.db or finances.json")
	to := flag.String("to", "", "v2 database to create (must not exist)")
	flag.Parse()
	if *from == "" || *to == "" {
		log.Fatal("usage: v1convert -from finance.db -to finance-v2.db")
	}
	if _, err := os.Stat(*to); err == nil {
		log.Fatalf("%s already exists", *to)
	}
	var src *importv1.Data
	var err error
	if strings.HasSuffix(*from, ".json") {
		raw, rerr := os.ReadFile(*from)
		if rerr != nil {
			log.Fatal(rerr)
		}
		src, err = importv1.ReadJSON(raw)
	} else {
		src, err = importv1.ReadDB(*from)
	}
	if err != nil {
		log.Fatal(err)
	}
	d, err := db.Open(*to)
	if err != nil {
		log.Fatal(err)
	}
	rep, err := importv1.Convert(d, src)
	if err != nil {
		log.Fatal(err)
	}
	out, _ := json.MarshalIndent(rep, "", "  ")
	os.Stdout.Write(out)
}
