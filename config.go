package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
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

	auth spotify.Authenticator
}

const (
	redirectURI = "http://localhost:8080/callback"
	state       = "fangirl"
)

func getTokenPath() (string, bool) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		// Better to not just error here, since we can technically still function.
		// But this does suck.
		return "", false
	}

	fangirlCacheDir := filepath.Join(cacheDir, "fangirl")
	if _, err := os.Stat(fangirlCacheDir); os.IsNotExist(err) {
		if err := os.Mkdir(fangirlCacheDir, 0755); err != nil {
			return "", false
		}
	}

	return filepath.Join(fangirlCacheDir, "token.txt"), true
}

// GetSpotifyClient retrieves the spotify client for the given invocation
// configuration.
func (c *Config) GetSpotifyClient() (*spotify.Client, error) {
	if c.cacheExists() {
		return c.getCachedSpotifyClient()
	}

	return c.getFreshSpotifyClient()
}

func (c *Config) cacheExists() bool {
	cacheDir, ok := getTokenPath()
	if !ok {
		log.Panicln("WARN: failed to find a cache directory for saving the oauth2 token")
		return false
	}

	_, err := os.Stat(cacheDir)

	return !os.IsNotExist(err)
}

func (c *Config) getCachedSpotifyClient() (*spotify.Client, error) {
	cacheDir, ok := getTokenPath()
	if !ok {
		return nil, errors.New("failed to find the cache dir for the oauth2 token")
	}

	tokenBytes, err := ioutil.ReadFile(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read the token file: %w", err)
	}

	token := oauth2.Token{}
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal the token bytes: %w", err)
	}

	// TODO: Should we be using token.Valid() to determine if we should actually
	// re-cache?
	client := c.auth.NewClient(&token)

	return &client, nil
}

func (c *Config) getFreshSpotifyClient() (*spotify.Client, error) {
	var clientChan = make(chan *spotify.Client)

	c.startHTTPServer(c.auth, clientChan)

	url := c.auth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

	// Wait for the auth flow to complete.
	client := <-clientChan

	user, err := client.CurrentUser()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("You are logged in as:", user.ID)

	token, err := client.Token()
	if err != nil {
		return client, fmt.Errorf("failed to retrieve token from the client for saving: %w", err)
	}

	c.saveToken(token)

	return client, nil
}

func (c *Config) saveToken(token *oauth2.Token) error {
	tokenBytes, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal the oauth2 token: %w", err)
	}

	cacheDir, ok := getTokenPath()
	if !ok {
		return errors.New("failed to find the cache dir for the oauth2 token")
	}

	if err := ioutil.WriteFile(cacheDir, tokenBytes, 0600); err != nil {
		return fmt.Errorf("failed to write the token file: %w", err)
	}

	return nil
}

// startHTTPServer and surrounding code is taken from the relevant examples
// from zmb3/spotify repository.
func (c *Config) startHTTPServer(auth spotify.Authenticator, clientChan chan *spotify.Client) {
	// Start an HTTP server on our callback URI, so that we can know when the
	// OAuth flow has completed.
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		tok, err := auth.Token(state, r)
		if err != nil {
			http.Error(w, "Couldn't get token", http.StatusForbidden)
			log.Fatal(err)
		}

		if st := r.FormValue("state"); st != state {
			http.NotFound(w, r)
			log.Fatalf("State mismatch: %s != %s\n", st, state)
		}

		// Use the token to get an authenticated client
		client := auth.NewClient(tok)
		fmt.Fprintf(w, "Login to fangirl completed!")
		clientChan <- &client
	})

	go http.ListenAndServe(":8080", nil)
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

	auth := spotify.NewAuthenticator(
		redirectURI,
		spotify.ScopeUserFollowRead,
		spotify.ScopeUserLibraryRead,
		spotify.ScopePlaylistModifyPrivate,
		spotify.ScopePlaylistReadPrivate,
	)
	auth.SetAuthInfo(spotifyClientID, spotifyClientSecret)
	return &Config{
		duration:     *durationPtr,
		playlistName: playlistName,
		runKind:      runKind,

		spotifyClientID:     spotifyClientID,
		spotifyClientSecret: spotifyClientSecret,

		auth: auth,
	}, nil
}
