package agent

import "testing"

func TestParseTarget(t *testing.T) {
	ws, err := ParseTarget("forge:7777")
	if err != nil || ws.Kind != "ws" || ws.WSURL != "ws://forge:7777/ws" {
		t.Fatalf("ws = %+v %v", ws, err)
	}
	ssh, err := ParseTarget("ssh://tushar@windows-box:2222")
	if err != nil || ssh.Kind != "ssh" || ssh.SSHDest != "tushar@windows-box" || ssh.SSHPort != "2222" {
		t.Fatalf("ssh = %+v %v", ssh, err)
	}
	httpURL, err := ParseTarget("http://127.0.0.1:7777")
	if err != nil || httpURL.WSURL != "ws://127.0.0.1:7777/ws" {
		t.Fatalf("http = %+v %v", httpURL, err)
	}
	if _, err := ParseTarget("not a target"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoteCommand(t *testing.T) {
	if got := RemoteCommand(""); got != "rigwatch agent --stdio" {
		t.Fatalf("cmd = %q", got)
	}
	if got := RemoteCommand(`/opt/Rig Watch/rigwatch`); !stringsHas(got, "agent --stdio") {
		t.Fatalf("cmd = %q", got)
	}
}

func stringsHas(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && contains(s, sub)))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
