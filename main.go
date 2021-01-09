package main

import (
	"fmt"
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

	log.Println("Fetching all followed artists")
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

		for _, artist := range followedArtists.Artists {
			artists = append(artists, artist.SimpleArtist)
			numArtists++
		}

		percentageDone := 100 * (float64(numArtists) / float64(followedArtists.Total))
		log.Printf("\tFetched %f%%", percentageDone)
		if numArtists >= followedArtists.Total {
			break
		}

		after = followedArtists.Cursor.After
	}

	log.Println("Fetched all followed artists")

	// TODO: Run on a subset for now. We should remove this later.
	artists = artists[:5]

	log.Println("Getting albums for artists")
	countryCode := "US"
	opts := spotify.Options{
		Country: &countryCode,
	}
	allAlbums := make([]spotify.SimpleAlbum, 0)
	// At this point we have a slice of artists. We want to, for each artist, get their albums.
	for _, artist := range artists {
		log.Printf("Getting albums for artist: %q", artist.Name)
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

		numAlbums := 0
		for {
			for _, album := range simpleAlbumPage.Albums {
				numAlbums++
				percentageDone := 100 * (float64(numAlbums) / float64(simpleAlbumPage.Total))
				log.Printf("\tFetched %f%%", percentageDone)
				allAlbums = append(allAlbums, album)
			}

			if err := client.NextPage(simpleAlbumPage); err == spotify.ErrNoMorePages {
				break
			} else if err != nil {
				log.Fatalf("failed to iterate to the next artist album page: %v", err)
			}

			// Unfortunately, we need to do this to avoid getting rate-limited by Spotify.
			time.Sleep(1 * time.Second)
		}
	}

	log.Println("Fetched albums for all artists")

	log.Println("Getting saved albums for user")
	// Before we get around to processing these albums we retrieved we need to
	// get the albums that the user has already liked. This is going to be useful
	// for determining if a released album has already been listened to by a
	// user.
	savedAlbumsPage, err := client.CurrentUsersAlbums()
	if err != nil {
		log.Fatalf("failed to get the saved albums: %v", err)
	}

	savedAlbums := make(map[string]spotify.SavedAlbum, 0)
	numAlbums := 0
	for {
		for _, album := range savedAlbumsPage.Albums {
			savedAlbums[album.ID.String()] = album
			numAlbums++
		}

		if err := client.NextPage(savedAlbumsPage); err == spotify.ErrNoMorePages {
			break
		} else if err != nil {
			log.Fatalf("failed to iterate to the next saved albums page: %v", err)
		}

		percentageDone := 100 * (float64(numAlbums) / float64(savedAlbumsPage.Total))
		log.Printf("\tFetched %f%%", percentageDone)
	}

	log.Println("Got saved albums")

	// We know that this is a strict subset of allAlbums, so it must have its
	// length or less.
	albums := make([]spotify.SimpleAlbum, 0, len(allAlbums))

	log.Println("Filtering albums")
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

	log.Println("Filtered albums")
	// At this point, we have all the albums we want to exist in our target playlist.
	for _, album := range albums {
		log.Printf("Album: %q by %s", album.Name, album.Artists[0].Name)
	}

	// So we're ready to potentially make, and append to a target playlist.
	currentUser, err := client.CurrentUser()
	if err != nil {
		log.Fatalf("failed to get the current user: %v", err)
	}

	sinceTime := time.Now().Add(-1 * cfg.duration)
	playlistTimeSuffix := sinceTime.Format("Jan _2, 2006")
	playlist, err := client.CreatePlaylistForUser(
		currentUser.ID,
		fmt.Sprintf("%s (%s)", cfg.playlistName, playlistTimeSuffix),
		fmt.Sprintf("Generated by fangirl - releases since %v", sinceTime.Format("Mon Jan _2, 3:04PM 2006")),
		false,
	)
	if err != nil {
		log.Fatalf("failed to create the playlist: %v", err)
	}

	for i, album := range albums {
		albumTracksPage, err := client.GetAlbumTracks(album.ID)
		if err != nil {
			log.Fatalf("failed to get album tracks: %v", err)
		}

		for {
			batch := make([]spotify.ID, 0, 100)
			for _, track := range albumTracksPage.Tracks {
				batch = append(batch, track.ID)
				if len(batch) == 100 {
					if _, err := client.AddTracksToPlaylist(playlist.ID, batch...); err != nil {
						log.Fatalf("failed to add tracks to the playlist: %v", err)
					}
					batch = batch[:0]
				}
			}

			if len(batch) != 0 {
				// If this happens, it means that we finished the batching loop above
				// and had some tracks leftover. So let's not forget to add this to the
				// playlist before we go to the next page of tracks.
				if _, err := client.AddTracksToPlaylist(playlist.ID, batch...); err != nil {
					log.Fatalf("failed to add tracks to the playlist: %v", err)
				}
			}

			if err := client.NextPage(albumTracksPage); err == spotify.ErrNoMorePages {
				break
			} else if err != nil {
				log.Fatalf("failed to iterate to the next album track page: %v", err)
			}
		}

		percentageDone := 100 * (float64(i+1) / float64(len(albums)))
		log.Printf("\tImported %f%%", percentageDone)
	}
}
