package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mattn/godcrawler"
	"log"
	"os"
)

func main() {
	nArgs := len(os.Args)
	if nArgs != 2 && nArgs != 3 {
		fmt.Println("Usage: godcrawler [db] [opml]")
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	crawler := godcrawler.New(db)

	if nArgs == 3 {
		f, err := os.Open(os.Args[2])
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		crawler.ImportOPML(f)
		f.Close()
	} else {
		crawler.Run()
	}
}
