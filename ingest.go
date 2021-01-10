package main

import (
	"fmt"
	"log"
	"time"

	"github.com/zmb3/spotify"
)

type ingester struct {
	client *spotify.Client
	cfg    *Config
}

type data struct {
	artists     []spotify.SimpleArtist
	albums      []spotify.SimpleAlbum
	savedAlbums map[string]spotify.SavedAlbum
}

func (in *ingester) Ingest() (*data, error) {
	log.Println("Fetching all followed artists")
	artists, err := in.getArtists()
	if err != nil {
		return nil, err
	}
	log.Println("Fetched all followed artists")

	log.Println("Getting albums for artists")
	allAlbums, err := in.getAlbumsForArtists(artists)
	if err != nil {
		return nil, err
	}
	log.Println("Fetched albums for all artists")

	log.Println("Getting saved albums for user")
	savedAlbums, err := in.getSavedAlbums()
	if err != nil {
		return nil, err
	}
	log.Println("Got saved albums")

	return &data{
		artists:     artists,
		albums:      allAlbums,
		savedAlbums: savedAlbums,
	}, nil
}

func (in *ingester) getArtists() ([]spotify.SimpleArtist, error) {
	// I didn't try super hard, but I also didn't find any better/cleaner way to
	// use this API because FullArtistCursorPage does not implement
	// spotify.pageable.
	after := ""
	numArtists := 0
	artists := make([]spotify.SimpleArtist, 0)
	for {
		followedArtists, err := in.client.CurrentUsersFollowedArtistsOpt(-1, after)
		if err != nil {
			return nil, fmt.Errorf("failed to get the followed artists: %w", err)
		}

		for _, artist := range followedArtists.Artists {
			numArtists++

			if _, ok := in.cfg.blacklistedArtists[artist.Name]; ok {
				log.Printf("\tSkipping blacklisted artist: %q", artist.Name)
				// If this is a blacklisted artist, then skip it.
				continue
			}

			artists = append(artists, artist.SimpleArtist)
		}

		percentageDone := 100 * (float64(numArtists) / float64(followedArtists.Total))
		log.Printf("\tFetched %f%%", percentageDone)
		if numArtists >= followedArtists.Total {
			break
		}

		after = followedArtists.Cursor.After
	}

	return artists, nil
}

func (in *ingester) getAlbumsForArtists(artists []spotify.SimpleArtist) ([]spotify.SimpleAlbum, error) {
	// TODO: Run on a subset for now. We should remove this later.
	artists = artists[:5]

	countryCode := "US"
	opts := spotify.Options{
		Country: &countryCode,
	}
	allAlbums := make([]spotify.SimpleAlbum, 0)
	// At this point we have a slice of artists. We want to, for each artist, get their albums.
	for i, artist := range artists {
		percentageDone := 100 * (float64(i) / float64(len(artists)))
		log.Printf("Getting albums for artist: %q (%f%% done)", artist.Name, percentageDone)
		simpleAlbumPage, err := in.client.GetArtistAlbumsOpt(
			artist.ID,
			&opts,
			spotify.AlbumTypeAlbum,
			spotify.AlbumTypeCompilation,
			spotify.AlbumTypeSingle,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get artist albums for %q: %w", artist.Name, err)
		}

		numAlbums := 0
		for {
			for _, album := range simpleAlbumPage.Albums {
				numAlbums++
				percentageDone := 100 * (float64(numAlbums) / float64(simpleAlbumPage.Total))
				log.Printf("\tFetched %f%%", percentageDone)
				allAlbums = append(allAlbums, album)
			}

			if err := in.client.NextPage(simpleAlbumPage); err == spotify.ErrNoMorePages {
				break
			} else if err != nil {
				return nil, fmt.Errorf("failed to iterate to the next artist album page: %w", err)
			}

			// Unfortunately, we need to do this to avoid getting rate-limited by Spotify.
			time.Sleep(1 * time.Second)
		}
	}

	return allAlbums, nil
}

func (in *ingester) getSavedAlbums() (map[string]spotify.SavedAlbum, error) {
	// Before we get around to processing these albums we retrieved we need to
	// get the albums that the user has already liked. This is going to be useful
	// for determining if a released album has already been listened to by a
	// user.
	savedAlbumsPage, err := in.client.CurrentUsersAlbums()
	if err != nil {
		return nil, fmt.Errorf("failed to get the saved albums: %w", err)
	}

	savedAlbums := make(map[string]spotify.SavedAlbum, 0)
	numAlbums := 0
	for {
		for _, album := range savedAlbumsPage.Albums {
			savedAlbums[album.ID.String()] = album
			numAlbums++
		}

		if err := in.client.NextPage(savedAlbumsPage); err == spotify.ErrNoMorePages {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to iterate to the next saved albums page: %w", err)
		}

		percentageDone := 100 * (float64(numAlbums) / float64(savedAlbumsPage.Total))
		log.Printf("\tFetched %f%%", percentageDone)
	}

	return savedAlbums, nil
}
