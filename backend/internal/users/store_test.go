package users_test

import (
	"context"
	"errors"
	"testing"

	"relay/internal/testdb"
	"relay/internal/users"
)

func newUser(t *testing.T, s *users.Store, username string) users.User {
	t.Helper()
	u, err := s.Create(context.Background(), users.NewUser{
		Email: username + "@example.com", Username: username, DisplayName: username, PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	return u
}

func TestStore_CreateAndGet(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	ctx := context.Background()
	u := newUser(t, s, "alice")

	byEmail, err := s.GetByEmail(ctx, "alice@example.com")
	if err != nil || byEmail.ID != u.ID {
		t.Fatalf("GetByEmail = %+v, %v", byEmail, err)
	}
	if _, err := s.GetByID(ctx, 999999); !errors.Is(err, users.ErrNotFound) {
		t.Errorf("GetByID(missing) err = %v, want ErrNotFound", err)
	}
}

func TestStore_UniqueEmailAndUsername(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	ctx := context.Background()
	newUser(t, s, "alice")

	_, err := s.Create(ctx, users.NewUser{Email: "alice@example.com", Username: "other", DisplayName: "x", PasswordHash: "x"})
	if !errors.Is(err, users.ErrEmailTaken) {
		t.Errorf("duplicate email err = %v", err)
	}
	_, err = s.Create(ctx, users.NewUser{Email: "other@example.com", Username: "alice", DisplayName: "x", PasswordHash: "x"})
	if !errors.Is(err, users.ErrUsernameTaken) {
		t.Errorf("duplicate username err = %v", err)
	}
}

func TestStore_SearchByPrefix(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	ctx := context.Background()
	me := newUser(t, s, "al_me")
	newUser(t, s, "alice")
	newUser(t, s, "al_bob")
	newUser(t, s, "bob")

	got, err := s.Search(ctx, "al", me.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if names := usernames(got); names != "al_bob,alice" {
		t.Errorf("search 'al' = %s (me must be excluded)", names)
	}

	got, _ = s.Search(ctx, "al_", 0, 20) // "_" must be literal, not a wildcard
	if names := usernames(got); names != "al_bob,al_me" {
		t.Errorf("search 'al_' = %s", names)
	}
}

func TestStore_GetMany(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	a, b := newUser(t, s, "alice"), newUser(t, s, "bob")

	got, err := s.GetMany(context.Background(), []int64{a.ID, b.ID, 424242})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[a.ID].Username != "alice" || got[b.ID].Username != "bob" {
		t.Errorf("GetMany = %+v", got)
	}
}

func TestStore_UpdateOnlyGivenFields(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	ctx := context.Background()
	u := newUser(t, s, "alice")
	newUser(t, s, "bob")

	bio := "hello"
	got, err := s.Update(ctx, u.ID, users.ProfileUpdate{Bio: &bio})
	if err != nil || got.Bio != "hello" || got.Username != "alice" {
		t.Fatalf("Update = %+v, %v", got, err)
	}
	taken := "bob"
	if _, err := s.Update(ctx, u.ID, users.ProfileUpdate{Username: &taken}); !errors.Is(err, users.ErrUsernameTaken) {
		t.Errorf("rename to taken username err = %v", err)
	}
}

func usernames(ps []users.Profile) string {
	out := ""
	for i, p := range ps {
		if i > 0 {
			out += ","
		}
		out += p.Username
	}
	return out
}

func TestStore_SetAvatarReturnsThePreviousKey(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	ctx := context.Background()
	u := newUser(t, s, "alice")

	k1, k2 := "avatars/1/a.jpg", "avatars/1/b.jpg"
	got, old, err := s.SetAvatar(ctx, u.ID, &k1)
	if err != nil || old != nil || got.AvatarURL == nil || *got.AvatarURL != "/media/avatars/1/a.jpg" {
		t.Fatalf("first = %+v old=%v err=%v", got.Profile, old, err)
	}
	_, old, _ = s.SetAvatar(ctx, u.ID, &k2)
	if old == nil || *old != k1 {
		t.Errorf("old = %v, want %s", old, k1)
	}
	got, old, _ = s.SetAvatar(ctx, u.ID, nil)
	if got.AvatarURL != nil || old == nil || *old != k2 {
		t.Errorf("remove: url=%v old=%v", got.AvatarURL, old)
	}
	many, _ := s.GetMany(ctx, []int64{u.ID})
	if many[u.ID].AvatarURL != nil {
		t.Error("GetMany still shows a removed picture")
	}
}
