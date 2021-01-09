package main

import (
	"log"
	"time"

	"github.com/zmb3/spotify"
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

	// I didn't try super hard, but I also didn't find any better/cleaner way to
	// use this API because FullArtistCursorPage does not implement
	// spotify.pageable.
	after := ""
	numArtists := 0
	artists := make([]spotify.SimpleArtist, 0)
	for {
		followedArtists, err := client.CurrentUsersFollowedArtistsOpt(-1, after)
		if err != nil {
			log.Fatalf("failed to get the followed artists: %v", err)
		}

		log.Println("Batch")
		for _, artist := range followedArtists.Artists {
			log.Printf("Followed artist: %q", artist.Name)
			artists = append(artists, artist.SimpleArtist)
			numArtists++
		}

		if numArtists >= followedArtists.Total {
			break
		}

		after = followedArtists.Cursor.After
		log.Println("AFTER: ", after)
	}

	countryCode := "US"
	opts := spotify.Options{
		Country: &countryCode,
	}
	allAlbums := make([]spotify.SimpleAlbum, 0)
	// At this point we have a slice of artists. We want to, for each artist, get their albums.
	for _, artist := range artists {
		simpleAlbumPage, err := client.GetArtistAlbumsOpt(artist.ID, &opts)
		if err != nil {
			log.Fatalf("failed to get artist albums for %q: %v", artist.Name, err)
		}

		for {
			for _, album := range simpleAlbumPage.Albums {
				allAlbums = append(allAlbums, album)
			}

			if err := client.NextPage(simpleAlbumPage); err == spotify.ErrNoMorePages {
				break
			}
		}
	}

	// At this point, we've effectively flat mapped the artists to a slice of albums.
	// Next, we want to filter out albums that are outside the duration we want.
	// Technically, we could have done this earlier in the above loop for
	// efficiency, but doing it here is nice because its much better organized.
	// This program does not give a damn about being ridiculously fast, it is
	// bottlenecked by Spotify API calls no matter how you look at it. An extra
	// in-memory loop won't hurt anyone.

	// We know that this is a strict subset of allAlbums, so it must have its
	// length or less.
	albums := make([]spotify.SimpleAlbum, 0, len(allAlbums))
	for _, album := range allAlbums {
		releaseTime := album.ReleaseDateTime()
		timeSinceRelease := time.Now().Sub(releaseTime)
		// If the time since it was released is less than the specified duration,
		// then the album was released in the the last `duration` time. Therefore,
		// this album qualifies and should pass the filter.
		if timeSinceRelease < cfg.duration {
			albums = append(albums, album)
		}
	}

	// At this point, we have all the albums we want to exist in our target playlist.
	for _, album := range albums {
		log.Printf("Album: %q by %s", album.Name, album.Artists[0].Name)
	}
}
