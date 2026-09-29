package main

import (
	"log"

	"semantic-search/ui"
)

func main() {
	log.Println("Starting Semantic Search App...")
	
	if err := ui.NewApp().Run(); err != nil {
		log.Fatal(err)
	}
}
