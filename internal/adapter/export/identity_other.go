//go:build !darwin && !linux

package export

import "os"

func openIdentityNoFollow(*os.Root, string) (*os.File, error) {
	return nil, invalidDestination()
}
