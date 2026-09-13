//go:build !nogui

package ui

import (
	_ "embed"

	"fyne.io/fyne/v2"

	"lolticker-agent/internal/agent"
)

// The four tray icons. They differ by silhouette, not only by colour: a tray
// renders them at 16-22 px against a background nobody controls, where a
// colour-only difference is invisible. Ring, disc, disc-with-a-notch and
// triangle stay tellable apart in greyscale.
//
//go:embed icons/offline.png
var iconOfflinePNG []byte

//go:embed icons/online.png
var iconOnlinePNG []byte

//go:embed icons/live.png
var iconLivePNG []byte

//go:embed icons/error.png
var iconErrorPNG []byte

var (
	resOffline = fyne.NewStaticResource("offline.png", iconOfflinePNG)
	resOnline  = fyne.NewStaticResource("online.png", iconOnlinePNG)
	resLive    = fyne.NewStaticResource("live.png", iconLivePNG)
	resError   = fyne.NewStaticResource("error.png", iconErrorPNG)
)

func iconForStatus(s agent.Status) fyne.Resource {
	switch s {
	case agent.StatusOnline:
		return resOnline
	case agent.StatusLive:
		return resLive
	case agent.StatusError:
		return resError
	default:
		return resOffline
	}
}
