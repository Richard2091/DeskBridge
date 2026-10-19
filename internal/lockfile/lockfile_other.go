//go:build !unix && !windows

package lockfile

import "github.com/Richard2091/DeskBridge/internal/platform"

func acquire(string) (func(), error) {
	return nil, platform.ErrUnsupportedPlatform
}
