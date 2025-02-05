package controller

import (
	"archive/zip"
	"context"
	"fmt"
	"image"
	"io"
	"log"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/dweymouth/supersonic/backend"
	"github.com/dweymouth/supersonic/backend/mediaprovider"
	"github.com/dweymouth/supersonic/backend/player"
	"github.com/dweymouth/supersonic/backend/player/mpv"
	"github.com/dweymouth/supersonic/sharedutil"
	"github.com/dweymouth/supersonic/ui/dialogs"
	"github.com/dweymouth/supersonic/ui/util"
	"github.com/dweymouth/supersonic/ui/widgets"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type NavigationHandler func(Route)

type ReloadFunc func()

type CurPageFunc func() Route

type Controller struct {
	AppVersion  string
	MainWindow  fyne.Window
	App         *backend.App
	NavHandler  NavigationHandler
	CurPageFunc CurPageFunc
	ReloadFunc  ReloadFunc

	escapablePopUp   *widget.PopUp
	haveModal        bool
	runOnModalClosed func()
}

func (m *Controller) NavigateTo(route Route) {
	m.NavHandler(route)
}

func (m *Controller) ClosePopUpOnEscape(pop *widget.PopUp) {
	m.escapablePopUp = pop
}

func (m *Controller) CloseEscapablePopUp() {
	if m.escapablePopUp != nil {
		m.escapablePopUp.Hide()
		m.escapablePopUp = nil
		m.doModalClosed()
	}
}

// If there is currently no modal popup managed by the Controller visible,
// then run f (which should create and show a modal dialog) immediately.
// else run f when the current modal dialog workflow has ended.
func (m *Controller) QueueShowModalFunc(f func()) {
	if m.haveModal {
		m.runOnModalClosed = f
	} else {
		f()
	}
}

func (m *Controller) HaveModal() bool {
	return m.haveModal
}

func (m *Controller) ShowPopUpImage(img image.Image) {
	im := canvas.NewImageFromImage(img)
	im.FillMode = canvas.ImageFillContain
	pop := widget.NewPopUp(im, m.MainWindow.Canvas())
	s := m.MainWindow.Canvas().Size()
	var popS fyne.Size
	if asp := util.ImageAspect(img); s.Width/s.Height > asp {
		// window height is limiting factor
		h := s.Height * 0.8
		popS = fyne.NewSize(h*asp, h)
	} else {
		w := s.Width * 0.8
		popS = fyne.NewSize(w, w*(1/asp))
	}
	m.ClosePopUpOnEscape(pop)
	pop.Resize(popS)
	pop.ShowAtPosition(fyne.NewPos(
		(s.Width-popS.Width)/2,
		(s.Height-popS.Height)/2,
	))
}

func (m *Controller) ConnectTracklistActionsWithReplayGainAlbum(tracklist *widgets.Tracklist) {
	m.connectTracklistActionsWithReplayGainMode(tracklist, player.ReplayGainAlbum)
}

func (m *Controller) ConnectTracklistActions(tracklist *widgets.Tracklist) {
	m.connectTracklistActionsWithReplayGainMode(tracklist, player.ReplayGainTrack)
}

func (m *Controller) connectTracklistActionsWithReplayGainMode(tracklist *widgets.Tracklist, mode player.ReplayGainMode) {
	tracklist.OnAddToPlaylist = m.DoAddTracksToPlaylistWorkflow
	tracklist.OnAddToQueue = func(tracks []*mediaprovider.Track) {
		m.App.PlaybackManager.LoadTracks(tracks, true, false)
	}
	tracklist.OnPlayTrackAt = func(idx int) {
		m.App.PlaybackManager.LoadTracks(tracklist.GetTracks(), false, false)
		if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
			m.App.PlaybackManager.SetReplayGainMode(mode)
		}
		m.App.PlaybackManager.PlayTrackAt(idx)
	}
	tracklist.OnPlaySelection = func(tracks []*mediaprovider.Track, shuffle bool) {
		m.App.PlaybackManager.LoadTracks(tracks, false, shuffle)
		if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
			m.App.PlaybackManager.SetReplayGainMode(mode)
		}
		m.App.PlaybackManager.PlayFromBeginning()
	}
	tracklist.OnSetFavorite = m.SetTrackFavorites
	tracklist.OnSetRating = m.SetTrackRatings
	tracklist.OnShowAlbumPage = func(albumID string) {
		m.NavigateTo(AlbumRoute(albumID))
	}
	tracklist.OnShowArtistPage = func(artistID string) {
		m.NavigateTo(ArtistRoute(artistID))
	}
	tracklist.OnColumnVisibilityMenuShown = func(pop *widget.PopUp) {
		m.ClosePopUpOnEscape(pop)
	}
	tracklist.OnDownload = m.ShowDownloadDialog
	tracklist.OnShare = func(trackID string) {
		go m.ShowShareDialog(trackID)
	}
	tracklist.OnPlaySongRadio = func(track *mediaprovider.Track) {
		go func() {
			tracks, err := m.GetSongRadioTracks(track)
			if err != nil {
				log.Println("Error getting song radio: ", err)
				return
			}
			m.App.PlaybackManager.LoadTracks(tracks, false, false)
			if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
				m.App.PlaybackManager.SetReplayGainMode(mode)
			}
			m.App.PlaybackManager.PlayFromBeginning()
		}()
	}
}

func (m *Controller) ConnectAlbumGridActions(grid *widgets.GridView) {
	grid.OnAddToQueue = func(albumID string) {
		go m.App.PlaybackManager.LoadAlbum(albumID, true, false)
	}
	grid.OnPlay = func(albumID string, shuffle bool) {
		go m.App.PlaybackManager.PlayAlbum(albumID, 0, shuffle)
	}
	grid.OnShowItemPage = func(albumID string) {
		m.NavigateTo(AlbumRoute(albumID))
	}
	grid.OnShowSecondaryPage = func(artistID string) {
		m.NavigateTo(ArtistRoute(artistID))
	}
	grid.OnAddToPlaylist = func(albumID string) {
		go func() {
			album, err := m.App.ServerManager.Server.GetAlbum(albumID)
			if err != nil {
				log.Printf("error loading album: %s", err.Error())
				return
			}
			m.DoAddTracksToPlaylistWorkflow(sharedutil.TracksToIDs(album.Tracks))
		}()
	}
	grid.OnDownload = func(albumID string) {
		go func() {
			album, err := m.App.ServerManager.Server.GetAlbum(albumID)
			if err != nil {
				log.Printf("error loading album: %s", err.Error())
				return
			}
			m.ShowDownloadDialog(album.Tracks, album.Name)
		}()
	}
	grid.OnShare = func(albumID string) {
		go m.ShowShareDialog(albumID)
	}
}

func (m *Controller) ConnectArtistGridActions(grid *widgets.GridView) {
	grid.OnShowItemPage = func(id string) { m.NavigateTo(ArtistRoute(id)) }
	grid.OnPlay = func(artistID string, shuffle bool) { go m.PlayArtistDiscography(artistID, shuffle) }
	grid.OnAddToQueue = func(artistID string) {
		go m.App.PlaybackManager.LoadTracks(m.GetArtistTracks(artistID), true /*append*/, false /*shuffle*/)
	}
	grid.OnAddToPlaylist = func(artistID string) {
		go m.DoAddTracksToPlaylistWorkflow(
			sharedutil.TracksToIDs(m.GetArtistTracks(artistID)))
	}
	grid.OnDownload = func(artistID string) {
		go func() {
			tracks := m.GetArtistTracks(artistID)
			artist, err := m.App.ServerManager.Server.GetArtist(artistID)
			if err != nil {
				log.Printf("error getting artist: %v", err.Error())
				return
			}
			m.ShowDownloadDialog(tracks, artist.Name)
		}()
	}
	grid.OnShare = func(artistID string) {
		go m.ShowShareDialog(artistID)
	}
}

func (m *Controller) GetArtistTracks(artistID string) []*mediaprovider.Track {
	server := m.App.ServerManager.Server
	if server == nil {
		log.Println("error playing artist discography: logged out")
		return nil
	}

	artist, err := server.GetArtist(artistID)
	if err != nil {
		log.Printf("error getting artist discography: %v", err.Error())
		return nil
	}
	var allTracks []*mediaprovider.Track
	for _, album := range artist.Albums {
		album, err := server.GetAlbum(album.ID)
		if err != nil {
			log.Printf("error loading album tracks: %v", err.Error())
			return nil
		}
		allTracks = append(allTracks, album.Tracks...)
	}
	return allTracks
}

func (m *Controller) PlayArtistDiscography(artistID string, shuffleTracks bool) {
	m.App.PlaybackManager.LoadTracks(m.GetArtistTracks(artistID), false, shuffleTracks)
	if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
		if shuffleTracks {
			m.App.PlaybackManager.SetReplayGainMode(player.ReplayGainTrack)
		} else {
			m.App.PlaybackManager.SetReplayGainMode(player.ReplayGainAlbum)
		}
	}
	m.App.PlaybackManager.PlayFromBeginning()
}

func (m *Controller) ShuffleArtistAlbums(artistID string) {
	artist, err := m.App.ServerManager.Server.GetArtist(artistID)
	if err != nil {
		log.Printf("error getting artist discography: %v", err.Error())
	}
	if len(artist.Albums) == 0 {
		return
	}

	rand.Shuffle(len(artist.Albums), func(i, j int) {
		artist.Albums[i], artist.Albums[j] = artist.Albums[j], artist.Albums[i]
	})
	m.App.PlaybackManager.StopAndClearPlayQueue()
	for _, al := range artist.Albums {
		m.App.PlaybackManager.LoadAlbum(al.ID, true /*append*/, false /*shuffle*/)
	}

	if m.App.Config.ReplayGain.Mode == backend.ReplayGainAuto {
		m.App.PlaybackManager.SetReplayGainMode(player.ReplayGainAlbum)
	}
	m.App.PlaybackManager.PlayFromBeginning()
}

func (m *Controller) PromptForFirstServer() {
	d := dialogs.NewAddEditServerDialog("Connect to Server", false, nil, m.MainWindow.Canvas().Focus)
	pop := widget.NewModalPopUp(d, m.MainWindow.Canvas())
	d.OnSubmit = func() {
		d.DisableSubmit()
		go func() {
			if m.testConnectionAndUpdateDialogText(d) {
				// connection is good
				pop.Hide()
				m.doModalClosed()
				conn := backend.ServerConnection{
					ServerType:  d.ServerType,
					Hostname:    d.Host,
					AltHostname: d.AltHost,
					Username:    d.Username,
					LegacyAuth:  d.LegacyAuth,
				}
				server := m.App.ServerManager.AddServer(d.Nickname, conn)
				if err := m.trySetPasswordAndConnectToServer(server, d.Password); err != nil {
					log.Printf("error connecting to server: %s", err.Error())
				}
			}
			d.EnableSubmit()
		}()
	}
	m.haveModal = true
	pop.Show()
}

// Show dialog to prompt for playlist.
// Depending on the results of that dialog, potentially create a new playlist
// Add tracks to the user-specified playlist
func (m *Controller) DoAddTracksToPlaylistWorkflow(trackIDs []string) {
	go func() {
		pls, err := m.App.ServerManager.Server.GetPlaylists()
		pls = sharedutil.FilterSlice(pls, func(pl *mediaprovider.Playlist) bool {
			return pl.Owner == m.App.ServerManager.LoggedInUser
		})
		if err != nil {
			// TODO: surface this error to user
			log.Printf("error getting user-owned playlists: %s", err.Error())
			return
		}

		selectedIdx := -1
		plNames := make([]string, 0, len(pls))
		for i, pl := range pls {
			plNames = append(plNames, pl.Name)
			if defId := m.App.Config.Application.DefaultPlaylistID; defId != "" && pl.ID == defId {
				selectedIdx = i
			}
		}

		dlg := dialogs.NewAddToPlaylistDialog("Add to Playlist", plNames, selectedIdx)
		pop := widget.NewModalPopUp(dlg, m.MainWindow.Canvas())
		m.ClosePopUpOnEscape(pop)
		dlg.OnCanceled = pop.Hide
		dlg.OnSubmit = func(playlistChoice int, newPlaylistName string) {
			pop.Hide()
			m.doModalClosed()
			if playlistChoice < 0 {
				go m.App.ServerManager.Server.CreatePlaylist(newPlaylistName, trackIDs)
			} else {
				playlist := pls[playlistChoice]
				m.App.Config.Application.DefaultPlaylistID = playlist.ID
				go m.App.ServerManager.Server.AddPlaylistTracks(
					playlist.ID, trackIDs)
			}
		}
		m.haveModal = true
		pop.Show()
	}()
}

func (m *Controller) DoEditPlaylistWorkflow(playlist *mediaprovider.Playlist) {
	canMakePublic := m.App.ServerManager.Server.CanMakePublicPlaylist()
	dlg := dialogs.NewEditPlaylistDialog(playlist, canMakePublic)
	pop := widget.NewModalPopUp(dlg, m.MainWindow.Canvas())
	m.ClosePopUpOnEscape(pop)
	dlg.OnCanceled = func() {
		pop.Hide()
		m.doModalClosed()
	}
	dlg.OnDeletePlaylist = func() {
		pop.Hide()
		dialog.ShowCustomConfirm("Confirm Delete Playlist", "OK", "Cancel", layout.NewSpacer(), /*custom content*/
			func(ok bool) {
				if !ok {
					pop.Show()
				} else {
					m.doModalClosed()
					go func() {
						if err := m.App.ServerManager.Server.DeletePlaylist(playlist.ID); err != nil {
							log.Printf("error deleting playlist: %s", err.Error())
						} else if rte := m.CurPageFunc(); rte.Page == Playlist && rte.Arg == playlist.ID {
							// navigate to playlists page if user is still on the page of the deleted playlist
							m.NavigateTo(PlaylistsRoute())
						}
					}()
				}
			}, m.MainWindow)
	}
	dlg.OnUpdateMetadata = func() {
		pop.Hide()
		m.doModalClosed()
		go func() {
			err := m.App.ServerManager.Server.EditPlaylist(playlist.ID, dlg.Name, dlg.Description, dlg.IsPublic)
			if err != nil {
				log.Printf("error updating playlist: %s", err.Error())
			} else if rte := m.CurPageFunc(); rte.Page == Playlist && rte.Arg == playlist.ID {
				// if user is on playlist page, reload to get the updates
				m.ReloadFunc()
			}
		}()
	}
	m.haveModal = true
	pop.Show()
}

// DoConnectToServerWorkflow does the workflow for connecting to the last active server on startup
func (c *Controller) DoConnectToServerWorkflow(server *backend.ServerConfig) {
	pass, err := c.App.ServerManager.GetServerPassword(server.ID)
	if err != nil {
		log.Printf("error getting password from keyring: %v", err)
		c.PromptForLoginAndConnect()
		return
	}

	// try connecting to last used server - set up cancelable modal dialog
	canceled := false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dlg := dialog.NewCustom("Connecting", "Cancel",
		widget.NewLabel(fmt.Sprintf("Connecting to %s", server.Nickname)), c.MainWindow)
	dlg.SetOnClosed(func() {
		canceled = true
		cancel()
	})
	c.haveModal = true
	dlg.Show()
	// try to connect
	if err := c.tryConnectToServer(ctx, server, pass); err != nil {
		dlg.Hide()
		c.haveModal = false
		if canceled {
			c.PromptForLoginAndConnect()
		} else {
			// connection failure
			dlg := dialog.NewError(err, c.MainWindow)
			dlg.SetOnClosed(func() {
				c.PromptForLoginAndConnect()
			})
			c.haveModal = true
			dlg.Show()
		}
	} else {
		dlg.Hide()
		c.haveModal = false
	}
}

func (m *Controller) PromptForLoginAndConnect() {
	d := dialogs.NewLoginDialog(m.App.Config.Servers, m.App.ServerManager.GetServerPassword)
	pop := widget.NewModalPopUp(d, m.MainWindow.Canvas())
	d.OnSubmit = func(server *backend.ServerConfig, password string) {
		d.DisableSubmit()
		d.SetInfoText("Testing connection...")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := m.App.ServerManager.TestConnectionAndAuth(ctx, server.ServerConnection, password)
			if err == backend.ErrUnreachable {
				d.SetErrorText("Server unreachable")
			} else if err != nil {
				d.SetErrorText("Authentication failed")
			} else {
				pop.Hide()
				m.trySetPasswordAndConnectToServer(server, password)
				m.doModalClosed()
			}
			d.EnableSubmit()
		}()
	}
	d.OnEditServer = func(server *backend.ServerConfig) {
		pop.Hide()
		editD := dialogs.NewAddEditServerDialog("Edit server", true, server, m.MainWindow.Canvas().Focus)
		editPop := widget.NewModalPopUp(editD, m.MainWindow.Canvas())
		editD.OnSubmit = func() {
			d.DisableSubmit()
			go func() {
				if m.testConnectionAndUpdateDialogText(editD) {
					// connection is good
					editPop.Hide()
					server.Hostname = editD.Host
					server.AltHostname = editD.AltHost
					server.Nickname = editD.Nickname
					server.Username = editD.Username
					server.LegacyAuth = editD.LegacyAuth
					m.trySetPasswordAndConnectToServer(server, editD.Password)
					m.doModalClosed()
				}
				d.EnableSubmit()
			}()
		}
		editD.OnCancel = func() {
			editPop.Hide()
			pop.Show()
		}
		editPop.Show()
	}
	d.OnNewServer = func() {
		pop.Hide()
		newD := dialogs.NewAddEditServerDialog("Add server", true, nil, m.MainWindow.Canvas().Focus)
		newPop := widget.NewModalPopUp(newD, m.MainWindow.Canvas())
		newD.OnSubmit = func() {
			d.DisableSubmit()
			go func() {
				if m.testConnectionAndUpdateDialogText(newD) {
					// connection is good
					newPop.Hide()
					conn := backend.ServerConnection{
						ServerType:  newD.ServerType,
						Hostname:    newD.Host,
						AltHostname: newD.AltHost,
						Username:    newD.Username,
						LegacyAuth:  newD.LegacyAuth,
					}
					server := m.App.ServerManager.AddServer(newD.Nickname, conn)
					m.trySetPasswordAndConnectToServer(server, newD.Password)
					m.doModalClosed()
				}
				d.EnableSubmit()
			}()
		}
		newD.OnCancel = func() {
			newPop.Hide()
			pop.Show()
		}
		newPop.Show()
	}
	d.OnDeleteServer = func(server *backend.ServerConfig) {
		pop.Hide()
		dialog.ShowConfirm("Confirm delete server",
			fmt.Sprintf("Are you sure you want to delete the server %q?", server.Nickname),
			func(ok bool) {
				if ok {
					m.App.ServerManager.DeleteServer(server.ID)
					m.App.DeleteServerCacheDir(server.ID)
					d.SetServers(m.App.Config.Servers)
				}
				if len(m.App.Config.Servers) == 0 {
					m.PromptForFirstServer()
				} else {
					pop.Show()
				}
			}, m.MainWindow)

	}
	m.haveModal = true
	pop.Show()
}

func (c *Controller) ShowAboutDialog() {
	dlg := dialogs.NewAboutDialog(c.AppVersion)
	pop := widget.NewModalPopUp(dlg, c.MainWindow.Canvas())
	dlg.OnDismiss = func() {
		pop.Hide()
		c.doModalClosed()
	}
	c.ClosePopUpOnEscape(pop)
	c.haveModal = true
	pop.Show()
}

func (c *Controller) ShowSettingsDialog(themeUpdateCallbk func(), themeFiles map[string]string) {
	devs, err := c.App.LocalPlayer.ListAudioDevices()
	if err != nil {
		log.Printf("error listing audio devices: %v", err)
		devs = []mpv.AudioDevice{{Name: "auto", Description: "Autoselect device"}}
	}

	curPlayer := c.App.PlaybackManager.CurrentPlayer()
	_, isReplayGainPlayer := curPlayer.(player.ReplayGainPlayer)
	_, isEqualizerPlayer := curPlayer.(*mpv.Player)
	isLocalPlayer := isEqualizerPlayer
	bands := c.App.LocalPlayer.Equalizer().BandFrequencies()
	dlg := dialogs.NewSettingsDialog(c.App.Config,
		devs, themeFiles, bands,
		c.App.ServerManager.Server.ClientDecidesScrobble(),
		isLocalPlayer, isReplayGainPlayer, isEqualizerPlayer,
		c.MainWindow)
	dlg.OnReplayGainSettingsChanged = func() {
		c.App.PlaybackManager.SetReplayGainOptions(c.App.Config.ReplayGain)
	}
	dlg.OnAudioExclusiveSettingChanged = func() {
		c.App.LocalPlayer.SetAudioExclusive(c.App.Config.LocalPlayback.AudioExclusive)
	}
	dlg.OnAudioDeviceSettingChanged = func() {
		c.App.LocalPlayer.SetAudioDevice(c.App.Config.LocalPlayback.AudioDeviceName)
	}
	dlg.OnThemeSettingChanged = themeUpdateCallbk
	dlg.OnEqualizerSettingsChanged = func() {
		// currently we only have one equalizer type
		eq := c.App.LocalPlayer.Equalizer().(*mpv.ISO15BandEqualizer)
		eq.Disabled = !c.App.Config.LocalPlayback.EqualizerEnabled
		eq.EQPreamp = c.App.Config.LocalPlayback.EqualizerPreamp
		copy(eq.BandGains[:], c.App.Config.LocalPlayback.GraphicEqualizerBands)
		c.App.LocalPlayer.SetEqualizer(eq)
	}
	pop := widget.NewModalPopUp(dlg, c.MainWindow.Canvas())
	dlg.OnDismiss = func() {
		pop.Hide()
		c.doModalClosed()
		c.App.SaveConfigFile()
	}
	c.ClosePopUpOnEscape(pop)
	c.haveModal = true
	pop.Show()
}

func (c *Controller) ShowQuickSearch() {
	qs := dialogs.NewQuickSearch(c.App.ServerManager.Server, c.App.ImageManager)
	pop := widget.NewModalPopUp(qs, c.MainWindow.Canvas())
	qs.OnDismiss = func() {
		pop.Hide()
		c.doModalClosed()
	}
	qs.OnNavigateTo = func(contentType mediaprovider.ContentType, id string) {
		pop.Hide()
		c.doModalClosed()
		switch contentType {
		case mediaprovider.ContentTypeAlbum:
			c.NavigateTo(AlbumRoute(id))
		case mediaprovider.ContentTypeArtist:
			c.NavigateTo(ArtistRoute(id))
		case mediaprovider.ContentTypeTrack:
			go c.App.PlaybackManager.PlayTrack(id)
		case mediaprovider.ContentTypePlaylist:
			c.NavigateTo(PlaylistRoute(id))
		case mediaprovider.ContentTypeGenre:
			c.NavigateTo(GenreRoute(id))
		}
	}
	c.ClosePopUpOnEscape(pop)
	c.haveModal = true
	min := qs.MinSize()
	height := fyne.Max(min.Height, fyne.Min(min.Height*1.5, c.MainWindow.Canvas().Size().Height*0.7))
	pop.Resize(fyne.NewSize(min.Width, height))
	pop.Show()
	c.MainWindow.Canvas().Focus(qs.SearchEntry)
}

func (c *Controller) trySetPasswordAndConnectToServer(server *backend.ServerConfig, password string) error {
	if err := c.App.ServerManager.SetServerPassword(server, password); err != nil {
		log.Printf("error setting keyring credentials: %v", err)
		// Don't return an error; fall back to just using the password in-memory
		// User will need to log in with the password on subsequent runs.
	}
	return c.tryConnectToServer(context.Background(), server, password)
}

// try to connect to the given server, with a 10 second timeout added to the context
func (c *Controller) tryConnectToServer(ctx context.Context, server *backend.ServerConfig, password string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.App.ServerManager.TestConnectionAndAuth(ctx, server.ServerConnection, password); err != nil {
		return err
	}
	if err := c.App.ServerManager.ConnectToServer(server, password); err != nil {
		log.Printf("error connecting to server: %v", err)
		return err
	}
	return nil
}

func (c *Controller) testConnectionAndUpdateDialogText(dlg *dialogs.AddEditServerDialog) bool {
	dlg.SetInfoText("Testing connection...")
	conn := backend.ServerConnection{
		ServerType:  dlg.ServerType,
		Hostname:    dlg.Host,
		AltHostname: dlg.AltHost,
		Username:    dlg.Username,
		LegacyAuth:  dlg.LegacyAuth,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.App.ServerManager.TestConnectionAndAuth(ctx, conn, dlg.Password)
	if err == backend.ErrUnreachable {
		dlg.SetErrorText("Could not reach server (wrong hostname?)")
		return false
	} else if err != nil {
		dlg.SetErrorText("Authentication failed (wrong username/password)")
		return false
	}
	return true
}

func (c *Controller) doModalClosed() {
	c.haveModal = false
	if c.runOnModalClosed != nil {
		c.runOnModalClosed()
		c.runOnModalClosed = nil
	}
}

func (c *Controller) SetTrackFavorites(trackIDs []string, favorite bool) {
	go c.App.ServerManager.Server.SetFavorite(mediaprovider.RatingFavoriteParameters{
		TrackIDs: trackIDs,
	}, favorite)

	for _, id := range trackIDs {
		c.App.PlaybackManager.OnTrackFavoriteStatusChanged(id, favorite)
	}
}

func (c *Controller) SetTrackRatings(trackIDs []string, rating int) {
	r, ok := c.App.ServerManager.Server.(mediaprovider.SupportsRating)
	if !ok {
		return
	}
	go r.SetRating(mediaprovider.RatingFavoriteParameters{
		TrackIDs: trackIDs,
	}, rating)

	// Notify PlaybackManager of rating change to update
	// the in-memory track models
	for _, id := range trackIDs {
		c.App.PlaybackManager.OnTrackRatingChanged(id, rating)
	}
}

func (c *Controller) ShowShareDialog(id string) {
	go func() {
		shareUrl, err := c.createShareURL(id)
		if err != nil {
			return
		}

		hyperlink := widget.NewHyperlink(shareUrl.String(), shareUrl)
		dlg := dialog.NewCustom("Share content", "OK",
			container.NewHBox(
				hyperlink,
				widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
					c.MainWindow.Clipboard().SetContent(hyperlink.Text)
				}),
				widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
					if shareUrl, err := c.createShareURL(id); err == nil {
						hyperlink.Text = shareUrl.String()
						hyperlink.URL = shareUrl
						hyperlink.Refresh()
					}
				}),
			),
			c.MainWindow,
		)
		dlg.Show()
	}()
}

func (c *Controller) createShareURL(id string) (*url.URL, error) {
	r, ok := c.App.ServerManager.Server.(mediaprovider.SupportsSharing)
	if !ok {
		return nil, fmt.Errorf("server does not support sharing")
	}

	shareUrl, err := r.CreateShareURL(id)
	if err != nil {
		log.Printf("error creating share URL: %v", err)
		c.showError(
			"Failed to share content. This commonly occurs when the server does not support sharing," +
				"or has the feature disabled.\nPlease check the server's settings and try again.",
		)
		return nil, err
	}
	return shareUrl, nil
}

func (c *Controller) ShowDownloadDialog(tracks []*mediaprovider.Track, downloadName string) {
	numTracks := len(tracks)
	var fileName string
	if numTracks == 1 {
		fileName = filepath.Base(tracks[0].FilePath)
	} else {
		fileName = "downloaded_tracks.zip"
	}

	dg := dialog.NewFileSave(
		func(file fyne.URIWriteCloser, err error) {
			if err != nil {
				log.Println(err)
				return
			}

			if file == nil {
				return
			}
			if numTracks == 1 {
				go c.downloadTrack(tracks[0], file.URI().Path())
			} else {
				go c.downloadTracks(tracks, file.URI().Path(), downloadName)
			}

		},
		c.MainWindow)
	dg.SetFileName(fileName)
	dg.Show()
}

func (c *Controller) downloadTrack(track *mediaprovider.Track, filePath string) {
	reader, err := c.App.ServerManager.Server.DownloadTrack(track.ID)
	if err != nil {
		log.Println(err)
		return
	}

	file, err := os.Create(filePath)
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("Saved song %s to: %s\n", track.Name, filePath)
	c.sendNotification(fmt.Sprintf("Download completed: %s", track.Name), fmt.Sprintf("Saved at: %s", filePath))
}

func (c *Controller) downloadTracks(tracks []*mediaprovider.Track, filePath, downloadName string) {
	zipFile, err := os.Create(filePath)
	if err != nil {
		log.Println(err)
		return
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, track := range tracks {
		reader, err := c.App.ServerManager.Server.DownloadTrack(track.ID)
		if err != nil {
			log.Println(err)
			continue
		}

		fileName := filepath.Base(track.FilePath)

		fileWriter, err := zipWriter.Create(fileName)
		if err != nil {
			log.Println(err)
			continue
		}

		_, err = io.Copy(fileWriter, reader)
		if err != nil {
			log.Println(err)
			continue
		}

		log.Printf("Saved song %s to: %s\n", track.Name, filePath)
	}

	c.sendNotification(fmt.Sprintf("Download completed: %s", downloadName), fmt.Sprintf("Saved at: %s", filePath))
}

func (c *Controller) sendNotification(title, content string) {
	fyne.CurrentApp().SendNotification(&fyne.Notification{
		Title:   title,
		Content: content,
	})
}

func (c *Controller) showError(content string) {
	// TODO: display an in-app toast message instead of a dialog.
	dialog.ShowError(fmt.Errorf(content), c.MainWindow)
}

func (c *Controller) ShowAlbumInfoDialog(albumID, albumName string, albumCover image.Image) {
	go func() {
		albumInfo, err := c.App.ServerManager.Server.GetAlbumInfo(albumID)
		if err != nil {
			log.Print("Error getting album info: ", err)
			return
		}
		dlg := dialogs.NewAlbumInfoDialog(albumInfo, albumName, albumCover)
		pop := widget.NewModalPopUp(dlg, c.MainWindow.Canvas())
		dlg.OnDismiss = func() {
			pop.Hide()
			c.doModalClosed()
		}
		c.ClosePopUpOnEscape(pop)
		c.haveModal = true
		pop.Show()
	}()
}

func (c *Controller) GetSongRadioTracks(sourceTrack *mediaprovider.Track) ([]*mediaprovider.Track, error) {
	radioTracks, err := c.App.ServerManager.Server.GetSongRadio(sourceTrack.ID, 100)
	if err != nil {
		return nil, fmt.Errorf("error getting song radio: %s", err.Error())
	}

	// The goal of this implementation is to place the source track first in the queue.
	filteredTracks := sharedutil.FilterSlice(radioTracks, func(track *mediaprovider.Track) bool {
		return track.ID != sourceTrack.ID
	})
	tracks := []*mediaprovider.Track{sourceTrack}
	tracks = append(tracks, filteredTracks...)
	return tracks, nil
}
