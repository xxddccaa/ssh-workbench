package hosts

import (
	"context"
	"testing"

	"sshworkbench/internal/fault"
)

type memoryRepository map[string]Profile

func (repository memoryRepository) ListHosts(context.Context) ([]Profile, error) {
	result := make([]Profile, 0, len(repository))
	for _, profile := range repository {
		result = append(result, profile)
	}
	return result, nil
}
func (repository memoryRepository) GetHost(_ context.Context, identifier string) (Profile, error) {
	profile, ok := repository[identifier]
	if !ok {
		return Profile{}, fault.New(404, "host_not_found", "missing")
	}
	return profile, nil
}
func (repository memoryRepository) PutHost(_ context.Context, profile Profile) error {
	repository[profile.ID] = profile
	return nil
}
func (repository memoryRepository) DeleteHost(_ context.Context, identifier string) error {
	delete(repository, identifier)
	return nil
}

func TestIdentityFileValidation(test *testing.T) {
	for _, value := range []string{"~/.ssh/../.bashrc", "~/.ssh/a/../../b", "~/.ssh/", "~/.ssh/x\x00", "relative/key"} {
		if ValidateIdentityFile(value) == nil {
			test.Errorf("accepted %q", value)
		}
	}
	for _, value := range []string{"~/.ssh/id_ed25519", "~/.ssh/work/中文 key", "~/key", "/etc/ssh/key"} {
		if err := ValidateIdentityFile(value); err != nil {
			test.Errorf("rejected %q: %v", value, err)
		}
	}
}

func TestJumpHostsAreSingleLevelAndProtected(test *testing.T) {
	ctx := context.Background()
	service := New(memoryRepository{})
	base := Profile{Hostname: "example.test", Username: "ops", AuthMode: AuthKeyboardInteractive}
	jump, err := service.Save(ctx, "", withName(base, "jump"))
	if err != nil {
		test.Fatal(err)
	}
	target := withName(base, "target")
	target.JumpHostID = jump.ID
	saved, err := service.Save(ctx, "", target)
	if err != nil {
		test.Fatal(err)
	}
	nested := withName(base, "nested")
	nested.JumpHostID = saved.ID
	if _, err := service.Save(ctx, "", nested); fault.As(err).Code != "nested_jump_host" {
		test.Fatalf("nested jump accepted: %v", err)
	}
	jump.JumpHostID = saved.ID
	if _, err := service.Save(ctx, jump.ID, jump); err == nil {
		test.Fatal("jump host got its own jump while in use")
	}
	self := saved
	self.JumpHostID = saved.ID
	if _, err := service.Save(ctx, saved.ID, self); fault.As(err).Code != "invalid_jump_host" {
		test.Fatal("self jump accepted")
	}
	if err := service.Delete(ctx, jump.ID); fault.As(err).Code != "host_in_use" {
		test.Fatal("deleted a jump host in use")
	}
	service.MarkUsed(ctx, saved.ID)
	if refreshed, _ := service.Get(ctx, saved.ID); refreshed.LastUsedAt == nil {
		test.Fatal("last use not recorded")
	}
}

func withName(profile Profile, name string) Profile {
	profile.Name = name
	return profile
}
