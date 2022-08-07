package main

import (
	"log"
	"time"

	"github.com/zmb3/spotify"
)

func filterData(d *data, duration time.Duration) *data {
	// We know that this is a strict subset of allAlbums, so it must have its
	// length or less.
	albums := make(map[string]spotify.SimpleAlbum, len(d.albums))

	log.Println("Filtering albums")
	// At this point, we've effectively flat mapped the artists to a slice of albums.
	// Next, we want to filter out albums that we don't want.
	// This means:
	//      Albums outside the duration.
	//      Albums the user has already liked.
	//  Duplicates (it is unclear sometimes why we get these from the Spotify API)
	// Technically, we could have done this earlier in the above loop for
	// efficiency, but doing it here is nice because its much better organized.
	// This program does not give a damn about being ridiculously fast, it is
	// bottlenecked by Spotify API calls no matter how you look at it. An extra
	// in-memory loop won't hurt anyone.
	for _, album := range d.albums {
		if _, ok := albums[album.ID.String()]; ok {
			// Skip albums we've seen already.
			// If we haven't seen it and the album passes the other filter conditions,
			// we'll remember it for next time.
			continue
		}

		releaseTime := album.ReleaseDateTime()
		timeSinceRelease := time.Now().Sub(releaseTime)
		// If the time since it was released is less than the specified duration,
		// then the album was released in the the last `duration` time. Therefore,
		// this album qualifies and should pass the filter.
		isRecent := timeSinceRelease < duration
		_, alreadySaved := d.savedAlbums[album.ID.String()]

		if isRecent && !alreadySaved {
			albums[album.ID.String()] = album
		}
	}

	// Now we need to convert albums to a slice:
	albumsSlice := make([]spotify.SimpleAlbum, 0, len(albums))
	for _, album := range albums {
		albumsSlice = append(albumsSlice, album)
	}

	log.Println("Filtered albums")

	return &data{
		albums:      albumsSlice,
		savedAlbums: d.savedAlbums,
		artists:     d.artists,
	}
}
