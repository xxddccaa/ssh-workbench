package auth

import (
	"fmt"
	"testing"
	"time"
)

func TestBootstrapIsSingleUseAndRevokesRequests(test *testing.T) {
	var revoked string
	service := New(false, "", func(owner string) { revoked = owner })
	defer service.Close()
	bootstrap := service.BootstrapToken()
	token, session, err := service.Exchange(bootstrap)
	if err != nil {
		test.Fatal(err)
	}
	if _, _, err := service.Exchange(bootstrap); err == nil {
		test.Fatal("bootstrap replay accepted")
	}
	if _, valid := service.Authenticate(token); !valid {
		test.Fatal("session invalid")
	}
	service.Revoke(token)
	if _, valid := service.Authenticate(token); valid {
		test.Fatal("revoked token still works")
	}
	if revoked != session.ID {
		test.Fatal("resources not revoked")
	}
	select {
	case <-session.Context.Done():
	case <-time.After(time.Second):
		test.Fatal("in-flight request not cancelled")
	}
}

func TestBootstrapExpiryAndRemoteMode(test *testing.T) {
	local := New(false, "", nil)
	defer local.Close()
	for digest := range local.bootstraps {
		local.bootstraps[digest] = time.Now().Add(-time.Second)
	}
	if _, _, err := local.Exchange(local.BootstrapToken()); err == nil {
		test.Fatal("expired bootstrap accepted")
	}
	if _, _, err := local.Exchange(""); err == nil {
		test.Fatal("empty bootstrap accepted")
	}
	reissued, err := local.IssueBootstrap()
	if err != nil {
		test.Fatal(err)
	}
	if _, _, err := local.Exchange(reissued); err != nil {
		test.Fatal("reissued bootstrap rejected", err)
	}
	if _, _, err := local.Exchange(reissued); err == nil {
		test.Fatal("reissued bootstrap replay accepted")
	}
	for range 8 {
		local.IssueBootstrap()
	}
	if _, err := local.IssueBootstrap(); err == nil {
		test.Fatal("unbounded outstanding bootstraps")
	}
	remote := New(true, "", nil)
	defer remote.Close()
	if _, _, err := remote.Exchange(""); err == nil {
		test.Fatal("remote bootstrap accepted")
	}
	if _, err := remote.IssueBootstrap(); err == nil {
		test.Fatal("remote mode issued a bootstrap")
	}
}

func TestPasswordHashAndRateLimit(test *testing.T) {
	password := "正确的密码-very-secret"
	hash, err := HashPassword(password)
	if err != nil {
		test.Fatal(err)
	}
	if !VerifyPassword(hash, password) || VerifyPassword(hash, "wrong") {
		test.Fatal("password verification broken")
	}
	if VerifyPassword("malformed", password) {
		test.Fatal("malformed hash accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		test.Fatal("short password accepted")
	}
	service := New(true, "malformed", nil)
	defer service.Close()
	for attempt := 0; attempt < 6; attempt++ {
		service.Login("127.0.0.1", "wrong")
	}
	if _, _, err := service.Login("127.0.0.1", password); err == nil {
		test.Fatal("rate limit missing")
	}
}

func TestManyAddressesCannotLockTheOwnerOut(test *testing.T) {
	password := "owner-password-long"
	hash, err := HashPassword(password)
	if err != nil {
		test.Fatal(err)
	}
	service := New(true, hash, nil)
	defer service.Close()
	for index := 0; index < 1100; index++ {
		service.mu.Lock()
		service.attempts[fmt.Sprintf("2001:db8::%x", index)] = attempt{start: time.Now(), count: 1}
		service.mu.Unlock()
	}
	if _, _, err := service.Login("192.0.2.10", password); err != nil {
		test.Fatalf("owner locked out by address rotation: %v", err)
	}
}
