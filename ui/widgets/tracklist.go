package widgets

import (
	"strconv"
	"supersonic/ui/layouts"
	"supersonic/ui/util"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/dweymouth/go-subsonic"
)

type TrackRow struct {
	widget.BaseWidget

	trackID       string
	prevTrackID   string
	prevIsPlaying bool

	num    *widget.RichText
	name   *widget.RichText
	artist *widget.RichText
	dur    *widget.RichText

	OnDoubleTapped func()

	container *fyne.Container
}

var _ fyne.DoubleTappable = (*TrackRow)(nil)

func NewTrackRow() *TrackRow {
	t := &TrackRow{}
	t.ExtendBaseWidget(t)
	t.num = widget.NewRichTextWithText("")
	t.name = widget.NewRichTextWithText("")
	t.name.Wrapping = fyne.TextTruncate
	t.artist = widget.NewRichTextWithText("")
	t.artist.Wrapping = fyne.TextTruncate
	t.dur = widget.NewRichTextWithText("")

	t.container = container.New(layouts.NewColumnsLayout([]float32{35, -1, -1, 55}),
		t.num, t.name, t.artist, t.dur)
	return t
}

func (t *TrackRow) Update(tr *subsonic.Child, isPlaying bool) {
	if tr.ID == t.prevTrackID && isPlaying == t.prevIsPlaying {
		return
	}
	t.prevTrackID = t.trackID
	t.prevIsPlaying = isPlaying
	t.trackID = tr.ID
	t.num.Segments[0].(*widget.TextSegment).Text = strconv.Itoa(tr.Track)
	t.name.Segments[0].(*widget.TextSegment).Text = tr.Title
	t.artist.Segments[0].(*widget.TextSegment).Text = tr.Artist
	t.dur.Segments[0].(*widget.TextSegment).Text = util.SecondsToTimeString(float64(tr.Duration))

	t.num.Segments[0].(*widget.TextSegment).Style.TextStyle = fyne.TextStyle{Bold: isPlaying}
	t.name.Segments[0].(*widget.TextSegment).Style.TextStyle = fyne.TextStyle{Bold: isPlaying, Italic: isPlaying}
	t.artist.Segments[0].(*widget.TextSegment).Style.TextStyle = fyne.TextStyle{Bold: isPlaying, Italic: isPlaying}
	t.dur.Segments[0].(*widget.TextSegment).Style.TextStyle = fyne.TextStyle{Bold: isPlaying}

	t.Refresh()
}

func (t *TrackRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.container)
}

func (t *TrackRow) DoubleTapped(*fyne.PointEvent) {
	if t.OnDoubleTapped != nil {
		t.OnDoubleTapped()
	}
}

type Tracklist struct {
	widget.BaseWidget

	Tracks        []*subsonic.Child
	OnPlayTrackAt func(int)

	nowPlayingIdx int
	list          *widget.List
}

func NewTracklist(tracks []*subsonic.Child) *Tracklist {
	t := &Tracklist{Tracks: tracks, nowPlayingIdx: -1}
	t.ExtendBaseWidget(t)
	t.list = widget.NewList(
		func() int { return len(t.Tracks) },
		func() fyne.CanvasObject { return NewTrackRow() },
		func(itemID widget.ListItemID, item fyne.CanvasObject) {
			tr := item.(*TrackRow)
			tr.OnDoubleTapped = func() { t.onPlayTrackAt(itemID) }
			tr.Update(t.Tracks[itemID], itemID == t.nowPlayingIdx)
		})
	return t
}

func (t *Tracklist) SetNowPlaying(trackID string) {
	t.nowPlayingIdx = -1
	for i, tr := range t.Tracks {
		if tr.ID == trackID {
			t.nowPlayingIdx = i
			break
		}
	}
	t.list.Refresh()
}

func (t *Tracklist) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.list)
}

func (t *Tracklist) onPlayTrackAt(idx int) {
	if t.OnPlayTrackAt != nil {
		t.OnPlayTrackAt(idx)
	}
}