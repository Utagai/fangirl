package main

import (
	"log"
	"time"
)

func main() {
	start := time.Now()

	cfg, err := getConfig()
	if err != nil {
		log.Fatalf("failed to initialize a configuration: %v", err)
	}

	log.Printf("Running with configuration: %s", cfg.String())

	client, err := cfg.getSpotifyClient()
	if err != nil {
		log.Fatalf("failed to get a Spotify API client: %v", err)
	}

	ingester := ingester{
		client: client,
		cfg:    cfg,
	}

	data, err := ingester.Ingest()
	if err != nil {
		log.Fatalf("failed to ingest data from Spotify: %v", err)
	}

	data = filterData(data, cfg.duration)

	// At this point, we have all the albums we want to exist in our target playlist.
	for _, album := range data.albums {
		log.Printf("Album: %q by %s", album.Name, album.Artists[0].Name)
	}

	if err := makePlaylist(client, cfg, data); err != nil {
		log.Fatalf("failed to create the playlist: %v", err)
	}

	end := time.Now()

	log.Printf("Added %d releases (out of %d artists) in %v", len(data.albums), len(data.artists), end.Sub(start))
}
