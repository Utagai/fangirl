package main

import (
	"log"
	"time"

	"github.com/zmb3/spotify"
)

func filterData(client *spotify.Client, d *data, duration time.Duration) *data {
	// We know that this is a strict subset of allAlbums, so it must have its
	// length or less.
	albums := make([]spotify.SimpleAlbum, 0, len(d.albums))

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
	for _, album := range d.albums {
		releaseTime := album.ReleaseDateTime()
		timeSinceRelease := time.Now().Sub(releaseTime)
		// If the time since it was released is less than the specified duration,
		// then the album was released in the the last `duration` time. Therefore,
		// this album qualifies and should pass the filter.
		isRecent := timeSinceRelease < duration
		_, alreadySaved := d.savedAlbums[album.ID.String()]

		if isRecent && !alreadySaved {
			albums = append(albums, album)
		}
	}

	log.Println("Filtered albums")

	return &data{
		albums:      albums,
		savedAlbums: d.savedAlbums,
		artists:     d.artists,
	}
}
