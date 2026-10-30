//go:build !windows

package power

func haltArgs() []string {
	return []string{"shutdown", "-h", "now"}
}
