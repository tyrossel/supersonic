package backend

import (
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/pelletier/go-toml/v2"
)

type ServerType string

const (
	ServerTypeSubsonic ServerType = "Subsonic"
	ServerTypeJellyfin ServerType = "Jellyfin"
)

type ServerConnection struct {
	ServerType  ServerType
	Hostname    string
	AltHostname string
	Username    string
	LegacyAuth  bool
}

type ServerConfig struct {
	ServerConnection
	ID       uuid.UUID
	Nickname string
	Default  bool
}

type AppConfig struct {
	WindowWidth                 int
	WindowHeight                int
	LastCheckedVersion          string
	LastLaunchedVersion         string
	EnableSystemTray            bool
	CloseToSystemTray           bool
	StartupPage                 string
	SettingsTab                 string
	AllowMultiInstance          bool
	MaxImageCacheSizeMB         int
	SavePlayQueue               bool
	DefaultPlaylistID           string
	ShowTrackChangeNotification bool

	// Experimental - may be removed in future
	FontNormalTTF string
	FontBoldTTF   string
	UIScaleSize   string
}

type AlbumPageConfig struct {
	TracklistColumns []string
}

type AlbumsPageConfig struct {
	SortOrder string
}

type ArtistPageConfig struct {
	InitialView      string
	TracklistColumns []string
}

type ArtistsPageConfig struct {
	SortOrder string
}

type FavoritesPageConfig struct {
	InitialView      string
	TracklistColumns []string
}

type PlaylistPageConfig struct {
	TracklistColumns []string
}

type PlaylistsPageConfig struct {
	InitialView string
}

type TracksPageConfig struct {
	TracklistColumns []string
}

type NowPlayingPageConfig struct {
	InitialView string
}

type LocalPlaybackConfig struct {
	AudioDeviceName       string
	AudioExclusive        bool
	InMemoryCacheSizeMB   int
	Volume                int
	EqualizerEnabled      bool
	EqualizerPreamp       float64
	GraphicEqualizerBands []float64
}

type ScrobbleConfig struct {
	Enabled              bool
	ThresholdTimeSeconds int
	ThresholdPercent     int
}

type ReplayGainConfig struct {
	Mode            string
	PreampGainDB    float64
	PreventClipping bool
}

type ThemeConfig struct {
	ThemeFile  string
	Appearance string
}

type TranscodingConfig struct {
	ForceRawFile bool
}

type Config struct {
	Application      AppConfig
	Servers          []*ServerConfig
	AlbumPage        AlbumPageConfig
	AlbumsPage       AlbumsPageConfig
	ArtistPage       ArtistPageConfig
	ArtistsPage      ArtistsPageConfig
	FavoritesPage    FavoritesPageConfig
	PlaylistPage     PlaylistPageConfig
	PlaylistsPage    PlaylistsPageConfig
	TracksPage       TracksPageConfig
	NowPlayingConfig NowPlayingPageConfig
	LocalPlayback    LocalPlaybackConfig
	Scrobbling       ScrobbleConfig
	ReplayGain       ReplayGainConfig
	Transcoding      TranscodingConfig
	Theme            ThemeConfig
}

var SupportedStartupPages = []string{"Albums", "Favorites", "Playlists"}

func DefaultConfig(appVersionTag string) *Config {
	return &Config{
		Application: AppConfig{
			WindowWidth:                 1000,
			WindowHeight:                800,
			LastCheckedVersion:          appVersionTag,
			LastLaunchedVersion:         "",
			EnableSystemTray:            true,
			CloseToSystemTray:           false,
			StartupPage:                 "Albums",
			SettingsTab:                 "General",
			AllowMultiInstance:          false,
			MaxImageCacheSizeMB:         50,
			UIScaleSize:                 "Normal",
			SavePlayQueue:               false,
			ShowTrackChangeNotification: false,
		},
		AlbumPage: AlbumPageConfig{
			TracklistColumns: []string{"Artist", "Time", "Plays", "Favorite", "Rating"},
		},
		AlbumsPage: AlbumsPageConfig{
			SortOrder: string("Recently Added"),
		},
		ArtistPage: ArtistPageConfig{
			InitialView:      "Discography",
			TracklistColumns: []string{"Album", "Time", "Plays", "Favorite", "Rating"},
		},
		ArtistsPage: ArtistsPageConfig{
			SortOrder: string("Name (A-Z)"),
		},
		FavoritesPage: FavoritesPageConfig{
			TracklistColumns: []string{"Artist", "Album", "Time", "Plays"},
			InitialView:      "Albums",
		},
		PlaylistPage: PlaylistPageConfig{
			TracklistColumns: []string{"Artist", "Album", "Time", "Plays"},
		},
		PlaylistsPage: PlaylistsPageConfig{
			InitialView: "List",
		},
		NowPlayingConfig: NowPlayingPageConfig{
			InitialView: "Play Queue",
		},
		TracksPage: TracksPageConfig{
			TracklistColumns: []string{"Artist", "Album", "Time", "Plays"},
		},
		LocalPlayback: LocalPlaybackConfig{
			// "auto" is the name to pass to MPV for autoselecting the output device
			AudioDeviceName:       "auto",
			AudioExclusive:        false,
			InMemoryCacheSizeMB:   30,
			Volume:                100,
			EqualizerEnabled:      false,
			EqualizerPreamp:       0,
			GraphicEqualizerBands: make([]float64, 15),
		},
		Scrobbling: ScrobbleConfig{
			Enabled:              true,
			ThresholdTimeSeconds: 240,
			ThresholdPercent:     50,
		},
		ReplayGain: ReplayGainConfig{
			Mode:            ReplayGainNone,
			PreampGainDB:    0.0,
			PreventClipping: true,
		},
		Transcoding: TranscodingConfig{
			ForceRawFile: false,
		},
		Theme: ThemeConfig{
			Appearance: "Dark",
		},
	}
}

func ReadConfigFile(filepath, appVersionTag string) (*Config, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := DefaultConfig(appVersionTag)
	if err := toml.NewDecoder(f).Decode(c); err != nil {
		return nil, err
	}

	// Backfill Subsonic to empty ServerType fields
	// for updating configs created before multiple MediaProviders were added
	for _, s := range c.Servers {
		if s.ServerType == "" {
			s.ServerType = ServerTypeSubsonic
		}
	}

	return c, nil
}

var writeLock sync.Mutex

func (c *Config) WriteConfigFile(filepath string) error {
	if !writeLock.TryLock() {
		return nil // another write in progress
	}
	defer writeLock.Unlock()

	b, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	os.WriteFile(filepath, b, 0644)

	return nil
}
