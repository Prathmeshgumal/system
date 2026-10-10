package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"short":                 false,
		"12345678":              true,
		strings.Repeat("a", 72): true,
		strings.Repeat("a", 73): false, // bcrypt ignores bytes after 72
		strings.Repeat("é", 37): false, // 37 characters but 74 bytes
	}
	for pw, ok := range cases {
		if msg := ValidatePassword(pw); (msg == "") != ok {
			t.Errorf("ValidatePassword(len %d bytes) = %q, want ok=%v", len(pw), msg, ok)
		}
	}
}

func TestHasher_HashAndVerify(t *testing.T) {
	h := NewHasher(bcrypt.MinCost, 1)
	hash, err := h.Hash(context.Background(), "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" || !strings.HasPrefix(hash, "$2a$04$") {
		t.Errorf("hash = %q, want a bcrypt hash with cost 04", hash)
	}
	if ok, _ := h.Verify(context.Background(), hash, "correct horse"); !ok {
		t.Error("right password rejected")
	}
	if ok, _ := h.Verify(context.Background(), hash, "wrong horse"); ok {
		t.Error("wrong password accepted")
	}
	again, _ := h.Hash(context.Background(), "correct horse")
	if again == hash {
		t.Error("same password gave the same hash — salt missing")
	}
}

func TestHasher_UnknownUserTakesAsLongAsKnownUser(t *testing.T) {
	h := NewHasher(10, 1) // slow enough to measure, fast enough for a test
	hash, _ := h.Hash(context.Background(), "password1")

	start := time.Now()
	h.Verify(context.Background(), hash, "nope-nope")
	known := time.Since(start)

	start = time.Now()
	h.VerifyNobody(context.Background(), "nope-nope")
	unknown := time.Since(start)

	if unknown < known/2 {
		t.Errorf("unknown email took %v vs %v for a known one — timing reveals which emails exist", unknown, known)
	}
}

// blockingHash replaces bcrypt in tests: it records how many hashes run at
// once and blocks until released.
type blockingHash struct {
	mu        sync.Mutex
	active    int
	maxActive int
	release   chan struct{}
}

func (b *blockingHash) gen(pw []byte, cost int) ([]byte, error) {
	b.mu.Lock()
	b.active++
	b.maxActive = max(b.maxActive, b.active)
	b.mu.Unlock()
	<-b.release
	b.mu.Lock()
	b.active--
	b.mu.Unlock()
	return []byte("hash"), nil
}

func TestHasher_LimitsConcurrentHashes(t *testing.T) {
	fake := &blockingHash{release: make(chan struct{})}
	h := NewHasher(bcrypt.MinCost, 2)
	h.generate = fake.gen

	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if _, err := h.Hash(context.Background(), "password1"); err != nil {
				t.Error(err)
			}
		})
	}
	time.Sleep(50 * time.Millisecond) // let all six try to start
	close(fake.release)
	wg.Wait()

	if fake.maxActive != 2 {
		t.Errorf("max concurrent hashes = %d, want 2 (the rest must queue)", fake.maxActive)
	}
}

func TestHasher_GivesUpWhenQueueTooLong(t *testing.T) {
	fake := &blockingHash{release: make(chan struct{})}
	defer close(fake.release)
	h := NewHasher(bcrypt.MinCost, 1)
	h.generate = fake.gen
	h.maxWait = 20 * time.Millisecond

	go h.Hash(context.Background(), "password1") // takes the only slot and holds it
	time.Sleep(10 * time.Millisecond)

	if _, err := h.Hash(context.Background(), "password2"); !errors.Is(err, ErrBusy) {
		t.Errorf("err = %v, want ErrBusy", err)
	}
}
