//go:build !windows && !linux

package web

func readSystemResources() systemResources {
	return systemResources{}
}
