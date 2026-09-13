//go:build !windows && !linux

package autostart

// macOS and anything else: a LaunchAgent plist is more work than it is worth
// at this stage, so the UI hides the checkbox rather than offering something
// that does nothing.
type unsupported struct{}

func newPlatform() Interface { return unsupported{} }

func (unsupported) Supported() bool          { return false }
func (unsupported) IsEnabled() (bool, error) { return false, ErrUnsupported }
func (unsupported) Enable() error            { return ErrUnsupported }
func (unsupported) Disable() error           { return ErrUnsupported }
