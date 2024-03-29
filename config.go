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
	"strings"
	"time"

	"github.com/zmb3/spotify"
	"golang.org/x/oauth2"
)

type config struct {
	duration           time.Duration
	playlistName       string
	blacklistedArtists map[string]struct{}

	spotifyClientID     string
	spotifyClientSecret string

	auth spotify.Authenticator
}

func (cfg *config) String() string {
	var sb strings.Builder

	// Yeah, this is kind of ugly. I don't care.
	sb.WriteString("{")
	sb.WriteString(fmt.Sprintf("duration: %v, ", cfg.duration))
	sb.WriteString(fmt.Sprintf("playlistName: %q, ", cfg.playlistName))
	blacklistedArtistsLst := make([]string, 0, len(cfg.blacklistedArtists))
	for artistName := range cfg.blacklistedArtists {
		blacklistedArtistsLst = append(blacklistedArtistsLst, artistName)
	}
	sb.WriteString(fmt.Sprintf("blacklistedArtists: [%s]", strings.Join(blacklistedArtistsLst, ", ")))
	sb.WriteString("}")

	return sb.String()
}

const (
	redirectURI = "http://localhost:8080/callback"
	state       = "fangirl"

	// Together, maxTries and retryDelay gives us a total wait time of
	// around 30 minutes. Sounds crazy, but this is a thing that runs in
	// a cronjob once a month and it really sucks if it fails the one
	// time it runs per month.

	// maxTries is the number of times we should retry a failed Spotify API request.
	maxTries = 60
	// retryDelay is the amount of time we should wait before retrying a failed Spotify API request.
	retryDelay = 30 * time.Second
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

func (cfg *config) getSpotifyClient() (*SpotifyClient, error) {
	if cfg.cacheExists() {
		client, err := cfg.getCachedSpotifyClient()
		if err != nil {
			return nil, err
		}
		return NewSpotifyClient(client, maxTries, retryDelay), err
	}

	client, err := cfg.getFreshSpotifyClient()
	if err != nil {
		return nil, err
	}

	return NewSpotifyClient(client, maxTries, retryDelay), err
}

func (cfg *config) cacheExists() bool {
	cacheDir, ok := getTokenPath()
	if !ok {
		log.Panicln("failed to find a cache directory for saving the oauth2 token")
		return false
	}

	_, err := os.Stat(cacheDir)

	return !os.IsNotExist(err)
}

func (cfg *config) getCachedSpotifyClient() (*spotify.Client, error) {
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
	client := cfg.auth.NewClient(&token)

	return &client, nil
}

func (cfg *config) getFreshSpotifyClient() (*spotify.Client, error) {
	var clientChan = make(chan *spotify.Client)

	cfg.startHTTPServer(cfg.auth, clientChan)

	url := cfg.auth.AuthURL(state)
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

	cfg.saveToken(token)

	return client, nil
}

func (cfg *config) saveToken(token *oauth2.Token) error {
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
func (cfg *config) startHTTPServer(auth spotify.Authenticator, clientChan chan *spotify.Client) {
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

func getConfig() (*config, error) {
	var playlistName string
	flag.StringVar(
		&playlistName,
		"playlist",
		"",
		"the name for the playlist containing recent releases",
	)

	durationPtr := flag.Duration(
		"duration",
		monthDuration,
		"the duration to consider 'recent'; defaults to 1 month",
	)

	var blacklistFile string
	flag.StringVar(
		&blacklistFile,
		"blacklist",
		"",
		"a path to a blacklist file containing artists to skip",
	)

	// Parse the command line arguments.
	flag.Parse()

	// If not supplied, default the playlist name to 'fangirl'.
	if playlistName == "" {
		playlistName = "fangirl"
	}

	var err error
	blacklistedArtists := map[string]struct{}{}
	if blacklistFile != "" {
		blacklistedArtists, err = getBlacklistedArtists(blacklistFile)
		if err != nil {
			return nil, fmt.Errorf("failed to get blacklisted artists: %w", err)
		}
	}

	auth := spotify.NewAuthenticator(
		redirectURI,
		spotify.ScopeUserFollowRead,
		spotify.ScopeUserLibraryRead,
		spotify.ScopePlaylistModifyPrivate,
		spotify.ScopePlaylistReadPrivate,
	)

	spotifyClientID, ok := os.LookupEnv("SPOTIFY_CLIENT_ID")
	if !ok {
		return nil, errors.New("SPOTIFY_CLIENT_ID environment variable is required to be set")
	}

	spotifyClientSecret, ok := os.LookupEnv("SPOTIFY_CLIENT_SECRET")
	if !ok {
		return nil, errors.New("SPOTIFY_CLIENT_SECRET environment variable is required to be set")
	}
	auth.SetAuthInfo(spotifyClientID, spotifyClientSecret)

	return &config{
		duration:           *durationPtr,
		playlistName:       playlistName,
		blacklistedArtists: blacklistedArtists,

		spotifyClientID:     spotifyClientID,
		spotifyClientSecret: spotifyClientSecret,

		auth: auth,
	}, nil
}

func getBlacklistedArtists(blacklistFile string) (map[string]struct{}, error) {
	fileContents, err := ioutil.ReadFile(blacklistFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read blacklist file: %w", err)
	}

	fileContentStr := string(fileContents)

	blacklistedArtists := make(map[string]struct{}, 0)
	for _, line := range strings.Split(fileContentStr, "\n") {
		trimmedLine := strings.Trim(line, "\n\r\t")
		if len(trimmedLine) != 0 {
			blacklistedArtists[trimmedLine] = struct{}{}
		}
	}

	return blacklistedArtists, nil
}
