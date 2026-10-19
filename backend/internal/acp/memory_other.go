//go:build !darwin && !linux

package acp

func memoryPressure() bool {
	return false
}
