package main

import (
	"context"
	"fmt"
	"os"

	"semantic-search/internal/indexer"
	"semantic-search/internal/search"
	"semantic-search/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	db, err := storage.NewDB("semantic-search.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.InitSchema(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init DB: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "index":
		if len(os.Args) < 3 {
			fmt.Println("Usage: search index <directory>")
			os.Exit(1)
		}
		idx := indexer.NewIndexer(db)
		if err := idx.IndexDir(context.Background(), os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "index failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Indexing complete")

	case "search":
		if len(os.Args) < 3 {
			fmt.Println("Usage: search search <query>")
			os.Exit(1)
		}
		s := search.NewSearcher(db)
		results, err := s.Search(context.Background(), os.Args[2], 10)
		if err != nil {
			fmt.Fprintf(os.Stderr, "search failed: %v\n", err)
			os.Exit(1)
		}
		for _, r := range results {
			fmt.Printf("%.2f %s\n  %s\n\n", r.Score, r.Title, r.Preview)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: search <command> [args]")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  index <directory>  Index PDF and MD files")
	fmt.Println("  search <query>    Search indexed files")
}
