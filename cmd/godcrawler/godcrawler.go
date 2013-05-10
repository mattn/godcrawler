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
	if len(os.Args) > 2 {
		fmt.Println("Usage: godcrawler [opml]")
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", "./godcrawler.db")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	crawler := godcrawler.New(db)

	if len(os.Args) == 2 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		f.Close()
		crawler.ImportOPML(f)
	} else {
		crawler.Run()
	}
}
