package agent

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Target is a machine a client can attach to.
type Target struct {
	Kind    string // ws or ssh
	Display string
	WSURL   string
	SSHDest string
	SSHPort string
	Raw     string
}

// ParseTarget accepts host:port, ws://, wss://, http://, https://, or ssh://user@host.
func ParseTarget(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, fmt.Errorf("empty target")
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return Target{}, fmt.Errorf("parse target: %w", err)
		}
		switch strings.ToLower(u.Scheme) {
		case "ssh":
			return parseSSH(u, raw)
		case "ws", "wss":
			return parseWS(u, raw, u.Scheme), nil
		case "http":
			u.Scheme = "ws"
			return parseWS(u, raw, "ws"), nil
		case "https":
			u.Scheme = "wss"
			return parseWS(u, raw, "wss"), nil
		default:
			return Target{}, fmt.Errorf("unknown scheme %q", u.Scheme)
		}
	}
	if _, _, err := net.SplitHostPort(raw); err != nil {
		return Target{}, fmt.Errorf("want host:port, ws://host:port, or ssh://user@host")
	}
	return Target{Kind: "ws", Display: raw, WSURL: "ws://" + raw + "/ws", Raw: raw}, nil
}

func parseSSH(u *url.URL, raw string) (Target, error) {
	host := u.Hostname()
	if host == "" {
		return Target{}, fmt.Errorf("ssh target missing host")
	}
	user := ""
	if u.User != nil {
		user = u.User.Username()
	}
	dest := host
	display := host
	if user != "" {
		dest = user + "@" + host
		display = dest
	}
	return Target{
		Kind:    "ssh",
		Display: display,
		SSHDest: dest,
		SSHPort: u.Port(),
		Raw:     raw,
	}, nil
}

func parseWS(u *url.URL, raw, scheme string) Target {
	if u.Path == "" || u.Path == "/" {
		u.Path = "/ws"
	}
	u.Scheme = scheme
	display := u.Host
	if display == "" {
		display = raw
	}
	return Target{Kind: "ws", Display: display, WSURL: u.String(), Raw: raw}
}

// RemoteCommand is the argv string OpenSSH runs on the far side.
func RemoteCommand(bin string) string {
	if strings.TrimSpace(bin) == "" {
		bin = "rigwatch"
	}
	return shellQuote(bin) + " agent --stdio"
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t\"'\\$") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
