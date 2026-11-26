package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/hosts"
)

func TestProfilesPersistWithoutSecretsAndPinsCannotBeReplaced(test *testing.T) {
	directory := filepath.Join(test.TempDir(), "private")
	store, err := Open(directory)
	if err != nil {
		test.Fatal(err)
	}
	service := hosts.New(store)
	profile, err := service.Save(context.Background(), "", hosts.Profile{Name: "中文主机", Hostname: "example.test", Username: "user"})
	if err != nil {
		test.Fatal(err)
	}
	key := connections.KnownKey{Address: profile.Address(), PublicKey: []byte{1}, Algorithm: "ssh-ed25519", Fingerprint: "first"}
	if err := store.AddKnownKey(context.Background(), key); err != nil {
		test.Fatal(err)
	}
	key.PublicKey = []byte{2}
	if err := store.AddKnownKey(context.Background(), key); err == nil {
		test.Fatal("replaced an existing host key")
	}
	store.Close()
	store, err = Open(directory)
	if err != nil {
		test.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.GetHost(context.Background(), profile.ID)
	if err != nil || loaded.Name != profile.Name || loaded.Port != 22 {
		test.Fatalf("profile did not round-trip: %+v %v", loaded, err)
	}
}
