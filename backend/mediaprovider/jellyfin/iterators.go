package jellyfin

import (
	"time"

	"github.com/dweymouth/go-jellyfin"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/mediaprovider/helpers"
	"github.com/dweymouth/supersonic/sharedutil"
)

const (
	AlbumSortRecentlyAdded  string = "Recently Added"
	AlbumSortRandom         string = "Random"
	AlbumSortTitleAZ        string = "Title (A-Z)"
	AlbumSortArtistAZ       string = "Artist (A-Z)"
	AlbumSortYearAscending  string = "Year (ascending)"
	AlbumSortYearDescending string = "Year (descending)"

	ArtistSortNameAZ string = "Name (A-Z)"
)

func (j *jellyfinMediaProvider) AlbumSortOrders() []string {
	return []string{
		AlbumSortRecentlyAdded,
		AlbumSortRandom,
		AlbumSortTitleAZ,
		AlbumSortArtistAZ,
		AlbumSortYearAscending,
		AlbumSortYearDescending,
	}
}

func (j *jellyfinMediaProvider) ArtistSortOrders() []string {
	return []string{
		ArtistSortNameAZ,
	}
}

func (j *jellyfinMediaProvider) IterateAlbums(sortOrder string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	var jfSort jellyfin.Sort
	switch sortOrder {
	case AlbumSortRecentlyAdded:
		jfSort.Field = jellyfin.SortByDateCreated
		jfSort.Mode = jellyfin.SortDesc
	case AlbumSortRandom:
		jfSort.Field = jellyfin.SortByRandom
	case AlbumSortArtistAZ:
		jfSort.Field = jellyfin.SortByArtist
		jfSort.Mode = jellyfin.SortAsc
	case AlbumSortTitleAZ:
		jfSort.Field = jellyfin.SortByName
		jfSort.Mode = jellyfin.SortAsc
	case AlbumSortYearAscending:
		jfSort.Field = jellyfin.SortByYear
		jfSort.Mode = jellyfin.SortAsc
	case AlbumSortYearDescending:
		jfSort.Field = jellyfin.SortByYear
		jfSort.Mode = jellyfin.SortDesc
	}
	jfFilt, modifiedFilter := jfFilterFromFilter(filter)

	fetcher := func(offs, limit int) ([]*mediaprovider.Album, error) {
		al, err := j.client.GetAlbums(jellyfin.QueryOpts{
			Sort:   jfSort,
			Filter: jfFilt,
			Paging: jellyfin.Paging{StartIndex: offs, Limit: limit},
		})
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(al, toAlbum), nil
	}

	if sortOrder == AlbumSortRandom {
		determFetcher := func(offs, limit int) ([]*mediaprovider.Album, error) {
			al, err := j.client.GetAlbums(jellyfin.QueryOpts{
				Sort:   jellyfin.Sort{Field: "SortName", Mode: jellyfin.SortAsc},
				Filter: jfFilt,
				Paging: jellyfin.Paging{StartIndex: offs, Limit: limit},
			})
			if err != nil {
				return nil, err
			}
			return sharedutil.MapSlice(al, toAlbum), nil
		}
		return helpers.NewRandomAlbumIter(determFetcher, fetcher, modifiedFilter, j.prefetchCoverCB)
	}
	return helpers.NewAlbumIterator(fetcher, modifiedFilter, j.prefetchCoverCB)
}

func (j *jellyfinMediaProvider) SearchAlbums(searchQuery string, filter mediaprovider.AlbumFilter) mediaprovider.AlbumIterator {
	fetcher := func(offs, limit int) ([]*mediaprovider.Album, error) {
		sr, err := j.client.Search(searchQuery, jellyfin.TypeAlbum, jellyfin.Paging{StartIndex: offs, Limit: limit})
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(sr.Albums, toAlbum), nil
	}
	return helpers.NewAlbumIterator(fetcher, filter, j.prefetchCoverCB)
}

func (j *jellyfinMediaProvider) IterateTracks(searchQuery string) mediaprovider.TrackIterator {
	var fetcher helpers.TrackFetchFn
	if searchQuery == "" {
		fetcher = func(offs, limit int) ([]*mediaprovider.Track, error) {
			var opts jellyfin.QueryOpts
			opts.Paging = jellyfin.Paging{StartIndex: offs, Limit: limit}
			s, err := j.client.GetSongs(opts)
			if err != nil {
				return nil, err
			}
			return sharedutil.MapSlice(s, toTrack), nil
		}
	} else {
		fetcher = func(offs, limit int) ([]*mediaprovider.Track, error) {
			sr, err := j.client.Search(searchQuery, jellyfin.TypeSong, jellyfin.Paging{StartIndex: offs, Limit: limit})
			if err != nil {
				return nil, err
			}
			return sharedutil.MapSlice(sr.Songs, toTrack), nil
		}
	}
	return helpers.NewTrackIterator(fetcher, j.prefetchCoverCB)
}

func (j *jellyfinMediaProvider) IterateArtists(sortOrder string, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	var jfSort jellyfin.Sort

	if sortOrder == "" {
		sortOrder = ArtistSortNameAZ // default
	}
	switch sortOrder {
	case ArtistSortNameAZ:
		jfSort.Field = jellyfin.SortByName
		jfSort.Mode = jellyfin.SortAsc
	}

	fetcher := func(offs, limit int) ([]*mediaprovider.Artist, error) {
		ar, err := j.client.GetAlbumArtists(jellyfin.QueryOpts{
			Sort:   jfSort,
			Paging: jellyfin.Paging{StartIndex: offs, Limit: limit},
		})
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(ar, toArtist), nil
	}

	return helpers.NewArtistIterator(fetcher, filter, j.prefetchCoverCB)
}

func (j *jellyfinMediaProvider) SearchArtists(searchQuery string, filter mediaprovider.ArtistFilter) mediaprovider.ArtistIterator {
	fetcher := func(offs, limit int) ([]*mediaprovider.Artist, error) {
		sr, err := j.client.Search(searchQuery, jellyfin.TypeArtist, jellyfin.Paging{StartIndex: offs, Limit: limit})
		if err != nil {
			return nil, err
		}
		return sharedutil.MapSlice(sr.Artists, toArtist), nil
	}
	return helpers.NewArtistIterator(fetcher, filter, j.prefetchCoverCB)
}

// Creates the Jellyfin filter to implement the given mediaprovider filter,
// and returns a modified mediaprovider filter, with now-unneeded fields zeroed out.
func jfFilterFromFilter(filter mediaprovider.AlbumFilter) (jellyfin.Filter, mediaprovider.AlbumFilter) {
	var jfFilt jellyfin.Filter

	// Clone the original filter to not modify its options.
	// Set filters must be maintained in the original filter, as they are used for the UI.
	// Modified filter options are used to ignore further filtering that was already handled by the
	// Jellyfin API.
	modifiedFilter := filter.Clone()
	filterOptions := modifiedFilter.Options()

	if filterOptions.ExcludeUnfavorited {
		jfFilt.Favorite = true
		filterOptions.ExcludeUnfavorited = false
	}
	if filterOptions.MinYear > 0 && filterOptions.MaxYear > 0 {
		jfFilt.YearRange = [2]int{filterOptions.MinYear, filterOptions.MaxYear}
		filterOptions.MinYear, filterOptions.MaxYear = 0, 0
	} else if filterOptions.MinYear > 0 {
		jfFilt.YearRange = [2]int{filterOptions.MinYear, time.Now().Year()}
		filterOptions.MinYear, filterOptions.MaxYear = 0, 0
	} else if filterOptions.MaxYear > 0 {
		jfFilt.YearRange = [2]int{1900, filterOptions.MaxYear}
		filterOptions.MinYear, filterOptions.MaxYear = 0, 0
	}
	jfFilt.Genres = filterOptions.Genres
	filterOptions.Genres = nil

	modifiedFilter.SetOptions(filterOptions)
	return jfFilt, modifiedFilter
}
