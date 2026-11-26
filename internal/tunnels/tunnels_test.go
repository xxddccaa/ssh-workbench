package tunnels

import "testing"

func TestTunnelsOnlyListenOnLoopback(test *testing.T) {
	valid := Spec{Name: "数据库", HostID: "h", ListenHost: "127.0.0.1", ListenPort: 15432, TargetHost: "db.internal", TargetPort: 5432}
	if err := Validate(valid); err != nil {
		test.Fatal(err)
	}
	for _, listen := range []string{"0.0.0.0", "::", "192.168.1.2", "localhost", ""} {
		candidate := valid
		candidate.ListenHost = listen
		if Validate(candidate) == nil {
			test.Errorf("listen host %q accepted", listen)
		}
	}
	for _, target := range []string{"", "a b", "user@host", "host/path", "fe80::1%eth0", "not:ipv6"} {
		candidate := valid
		candidate.TargetHost = target
		if Validate(candidate) == nil {
			test.Errorf("target %q accepted", target)
		}
	}
	candidate := valid
	candidate.ListenPort = 0
	if Validate(candidate) == nil {
		test.Error("port 0 accepted")
	}
}
