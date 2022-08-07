package main

import (
	"time"

	"github.com/zmb3/spotify"
)

// SpotifyClient is a wrapper around spotify.Client that adds retry logic.
// Note that spotify.Client has an AutoRetry flag that one can set
// true, and this struct does indeed set that flag, but this only
// catches certain HTTP codes that indicate a retry may help, namely,
// 429 (Too Many Requests).
// What we want is to _also_ retry on any errors, like e.g. a 502 (Bad
// Gateway), which Spotify's API does indeed return sometimes.
// This is especially important for fangirl in particular because its
// execution times are so long (increasing the likelihood that it runs
// into a failure of Spotify's API, even if its SLA is great!).
type SpotifyClient struct {
	client     *spotify.Client
	maxTries   uint
	retryDelay time.Duration
}

func NewSpotifyClient(client *spotify.Client, maxTries uint, retryDelay time.Duration) *SpotifyClient {
	client.AutoRetry = true
	return &SpotifyClient{
		client:     client,
		maxTries:   maxTries,
		retryDelay: retryDelay,
	}
}

func wrapInRetry(fun func() error, maxTries uint, retryDelay time.Duration) (err error) {
	for i := uint(0); i <= maxTries; i++ {
		err = fun()
		if err != nil {
			if i < maxTries {
				time.Sleep(retryDelay)
			}
			continue
		}

		break
	}

	return err
}

func wrapInRetryWithRet[T any](fun func() (T, error), maxTries uint, retryDelay time.Duration) (ret T, err error) {
	// Maybe this is not that readable with the variable
	// shadowing... but damn that was cool to write.
	return ret, wrapInRetry(func() error {
		ret, err = fun()
		return err
	}, maxTries, retryDelay)
}

func (sc *SpotifyClient) CurrentUsersFollowedArtistsOpt(limit int, after string) (*spotify.FullArtistCursorPage, error) {
	return wrapInRetryWithRet(func() (*spotify.FullArtistCursorPage, error) {
		return sc.client.CurrentUsersFollowedArtistsOpt(limit, after)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) GetArtistAlbumsOpt(artistID spotify.ID, options *spotify.Options, ts ...spotify.AlbumType) (*spotify.SimpleAlbumPage, error) {
	return wrapInRetryWithRet(func() (*spotify.SimpleAlbumPage, error) {
		return sc.client.GetArtistAlbumsOpt(artistID, options, ts...)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) CurrentUsersAlbums() (*spotify.SavedAlbumPage, error) {
	return wrapInRetryWithRet(func() (*spotify.SavedAlbumPage, error) {
		return sc.client.CurrentUsersAlbums()
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) CreatePlaylistForUser(userID string, playlistName string, description string, public bool) (*spotify.FullPlaylist, error) {
	return wrapInRetryWithRet(func() (*spotify.FullPlaylist, error) {
		return sc.client.CreatePlaylistForUser(userID, playlistName, description, public)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) GetAlbumTracks(id spotify.ID) (*spotify.SimpleTrackPage, error) {
	return wrapInRetryWithRet(func() (*spotify.SimpleTrackPage, error) {
		return sc.client.GetAlbumTracks(id)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) AddTracksToPlaylist(playlistID spotify.ID, trackIDs ...spotify.ID) (string, error) {
	return wrapInRetryWithRet(func() (string, error) {
		return sc.client.AddTracksToPlaylist(playlistID, trackIDs...)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) CurrentUser() (*spotify.PrivateUser, error) {
	return wrapInRetryWithRet(func() (*spotify.PrivateUser, error) {
		return sc.client.CurrentUser()
	}, sc.maxTries, sc.retryDelay)
}

// The following functions are unfortunately necessary because
// spotify.Client#NextPage() takes a spotify.pageable, but since this
// is not exported, we can't create a wrapping
// SpotifyClient#NextPage() implementation.
func (sc *SpotifyClient) NextSimpleAlbumPage(albumPage *spotify.SimpleAlbumPage) error {
	return wrapInRetry(func() error {
		return sc.client.NextPage(albumPage)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) NextSavedAlbumPage(albumPage *spotify.SavedAlbumPage) error {
	return wrapInRetry(func() error {
		return sc.client.NextPage(albumPage)
	}, sc.maxTries, sc.retryDelay)
}

func (sc *SpotifyClient) NextSimpleTrackPage(trackPage *spotify.SimpleTrackPage) error {
	return wrapInRetry(func() error {
		return sc.client.NextPage(trackPage)
	}, sc.maxTries, sc.retryDelay)
}
