//go:build linux

package netstack

var stackBuilders = []NewStackFunc{
	NewStackChannel,
	NewStackFdbased,
}
