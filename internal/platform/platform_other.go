//go:build !windows && !linux && !darwin

package platform

func desktopHelperName() (string, error) {
	return "", ErrUnsupportedPlatform
}
