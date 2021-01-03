package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2/clientcredentials"
)

// RunKind is the kind of task a fangirl invocation should do.
type RunKind int

const (
	// RecentReleases finds all the unliked albums of followed artists released in the last month.
	RecentReleases RunKind = iota
	// AllUnliked finds _all_ the unliked albums of followed artists, prior to now - duration.
	AllUnliked
)

// Config is the configuration used during a fangirl invocation to determine
// what and how it needs to work with the user's playlists.
type Config struct {
	duration     time.Duration
	playlistName string
	runKind      RunKind

	spotifyClientID     string
	spotifyClientSecret string
}

// GetSpotifyClient retrieves the spotify client for the given invocation
// configuration.
func (c *Config) GetSpotifyClient() (*spotify.Client, error) {
	config := &clientcredentials.Config{
		ClientID:     c.spotifyClientID,
		ClientSecret: c.spotifyClientSecret,
		TokenURL:     spotify.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve the token: %w", err)
	}

	client := spotify.Authenticator{}.NewClient(token)

	return &client, nil
}

// Obviously there is no constant value that can express the length of a month,
// but we assume every month is 31 days. It really doesn'tt matter.
const monthDuration = time.Hour * 24 * 31

// GetConfig is a constructor for a Config. It initializes the fields
// as best as it can.  It will error if it runs into anything considered
// invalid for a fangirl invocation.
func GetConfig() (*Config, error) {
	spotifyClientID, ok := os.LookupEnv("SPOTIFY_CLIENT_ID")
	if !ok {
		return nil, errors.New("SPOTIFY_CLIENT_ID environment variable is required to be set")
	}

	spotifyClientSecret, ok := os.LookupEnv("SPOTIFY_CLIENT_SECRET")
	if !ok {
		return nil, errors.New("SPOTIFY_CLIENT_SECRET environment variable is required to be set")
	}

	var unlikedPriorToDurationPlaylistName string
	flag.StringVar(
		&unlikedPriorToDurationPlaylistName,
		"unliked",
		"",
		"the name for the playlist that will contain all unliked songs",
	)

	var recentReleasesPlaylistName string
	flag.StringVar(
		&recentReleasesPlaylistName,
		"recent",
		"",
		"the name for the playlist containing recent releases",
	)

	durationPtr := flag.Duration(
		"duration",
		monthDuration,
		"the duration to consider 'recent'; defaults to 1 month",
	)

	// Parse the command line arguments.
	flag.Parse()

	// Both of these cannot be supplied simultaneously for a single invocation.
	if unlikedPriorToDurationPlaylistName != "" && recentReleasesPlaylistName != "" {
		return nil, errors.New("cannot specify both unliked and recent in the same invocation")
	}

	// But, at least one _must_ be supplied.
	if unlikedPriorToDurationPlaylistName == "" && recentReleasesPlaylistName == "" {
		return nil, errors.New("must specify either unliked or recent for an invocation")
	}

	runKind := RecentReleases
	playlistName := recentReleasesPlaylistName
	if unlikedPriorToDurationPlaylistName != "" {
		runKind = AllUnliked
		playlistName = unlikedPriorToDurationPlaylistName
	}

	return &Config{
		duration:     *durationPtr,
		playlistName: playlistName,
		runKind:      runKind,

		spotifyClientID:     spotifyClientID,
		spotifyClientSecret: spotifyClientSecret,
	}, nil
}

func main() {
	cfg, err := GetConfig()
	fmt.Println("CFG: ", cfg)
	fmt.Println("ERR: ", err)
	if err != nil {
		log.Fatalf("failed to initialize a configuration: %v", err)
	}

	fmt.Printf("CFG: %v\n", cfg)

	client, err := cfg.GetSpotifyClient()
	if err != nil {
		log.Fatalf("failed to get a Spotify API client: %v", err)
	}

	tracks, err := client.GetPlaylistTracks("1lmPXgXRgkfDlnA65CcvNC")
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Playlist has %d total tracks", tracks.Total)
	for page := 1; ; page++ {
		log.Printf("  Page %d has %d tracks", page, len(tracks.Tracks))
		err = client.NextPage(tracks)
		if err == spotify.ErrNoMorePages {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
	}
}
