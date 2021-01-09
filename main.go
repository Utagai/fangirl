package main

import (
	"log"
)

func main() {
	cfg, err := GetConfig()
	if err != nil {
		log.Fatalf("failed to initialize a configuration: %v", err)
	}

	client, err := cfg.GetSpotifyClient()
	if err != nil {
		log.Fatalf("failed to get a Spotify API client: %v", err)
	}

	ingester := ingester{
		client: client,
	}

	data, err := ingester.Ingest()
	if err != nil {
		log.Fatalf("failed to ingest data from Spotify: %v", err)
	}

	data = filterData(client, data, cfg.duration)

	// At this point, we have all the albums we want to exist in our target playlist.
	for _, album := range data.albums {
		log.Printf("Album: %q by %s", album.Name, album.Artists[0].Name)
	}

	if err := makePlaylist(client, cfg, data); err != nil {
		log.Fatalf("failed to create the playlist: %v", err)
	}
}
