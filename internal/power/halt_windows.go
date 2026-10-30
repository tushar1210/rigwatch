//go:build windows

package power

func haltArgs() []string {
	return []string{"shutdown", "/s", "/t", "0", "/f"}
}
