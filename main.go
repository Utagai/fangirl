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

	followedArtists, err := client.CurrentUsersFollowedArtists()
	if err != nil {
		log.Fatalf("failed to get the followed artists: %v", err)
	}

	log.Printf("User has %d total followedArtists", followedArtists.Total)

	for _, artist := range followedArtists.Artists {
		log.Printf("Followed artist: %q", artist.Name)
	}
}
