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

	artists = artists[:100]

	countryCode := "US"
	opts := spotify.Options{
		Country: &countryCode,
	}
	allAlbums := make([]spotify.SimpleAlbum, 0)
	// At this point we have a slice of artists. We want to, for each artist, get their albums.
	for _, artist := range artists {
		simpleAlbumPage, err := client.GetArtistAlbumsOpt(
			artist.ID,
			&opts,
			spotify.AlbumTypeAlbum,
			spotify.AlbumTypeCompilation,
			spotify.AlbumTypeSingle,
		)
		if err != nil {
			log.Fatalf("failed to get artist albums for %q: %v", artist.Name, err)
		}

		for {
			for _, album := range simpleAlbumPage.Albums {
				allAlbums = append(allAlbums, album)
			}

			if err := client.NextPage(simpleAlbumPage); err == spotify.ErrNoMorePages {
				break
			} else if err != nil {
				log.Fatalf("failed to iterate to the next artist album page: %v", err)
			}
		}
	}

	// Before we get around to processing these albums we retrieved we need to
	// get the albums that the user has already liked. This is going to be useful
	// for determining if a released album has already been listened to by a
	// user.
	savedAlbumsPage, err := client.CurrentUsersAlbums()
	if err != nil {
		log.Fatalf("failed to get the saved albums: %v", err)
	}

	savedAlbums := make(map[string]spotify.SavedAlbum, 0)
	for {
		for _, album := range savedAlbumsPage.Albums {
			savedAlbums[album.ID.String()] = album
		}

		if err := client.NextPage(savedAlbumsPage); err == spotify.ErrNoMorePages {
			break
		} else if err != nil {
			log.Fatalf("failed to iterate to the next saved albums page: %v", err)
		}
	}

	// We know that this is a strict subset of allAlbums, so it must have its
	// length or less.
	albums := make([]spotify.SimpleAlbum, 0, len(allAlbums))

	// At this point, we've effectively flat mapped the artists to a slice of albums.
	// Next, we want to filter out albums that we don't want.
	// This means:
	//	Albums outside the duration.
	//	Albums the user has already liked.
	// Technically, we could have done this earlier in the above loop for
	// efficiency, but doing it here is nice because its much better organized.
	// This program does not give a damn about being ridiculously fast, it is
	// bottlenecked by Spotify API calls no matter how you look at it. An extra
	// in-memory loop won't hurt anyone.
	for _, album := range allAlbums {
		releaseTime := album.ReleaseDateTime()
		timeSinceRelease := time.Now().Sub(releaseTime)
		// If the time since it was released is less than the specified duration,
		// then the album was released in the the last `duration` time. Therefore,
		// this album qualifies and should pass the filter.
		isRecent := timeSinceRelease < cfg.duration
		_, alreadySaved := savedAlbums[album.ID.String()]

		if isRecent && !alreadySaved {
			albums = append(albums, album)
		}
	}

	// At this point, we have all the albums we want to exist in our target playlist.
	for _, album := range albums {
		log.Printf("Album: %q by %s", album.Name, album.Artists[0].Name)
	}
}
