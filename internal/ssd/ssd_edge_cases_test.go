package ssd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAppendReadBinaryAndEmptyValues(t *testing.T) {
	t.Parallel()

	disk, err := New(0, t.TempDir(), Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	cases := []struct {
		key   string
		value []byte
	}{
		{key: "empty", value: []byte{}},
		{key: "binary", value: []byte{0, 1, 2, '\n', 255}},
		{key: "unicode", value: []byte("hello 🌎")},
	}

	for _, tc := range cases {
		lba, err := disk.Append(tc.key, tc.value)
		if err != nil {
			t.Fatalf("Append(%q) error = %v", tc.key, err)
		}
		got, err := disk.Read(lba)
		if err != nil {
			t.Fatalf("Read(%d) error = %v", lba, err)
		}
		if !bytes.Equal(got, tc.value) {
			t.Fatalf("Read(%q) = %v, want %v", tc.key, got, tc.value)
		}
	}
}

func TestConcurrentAppendAllocatesUniqueLBAs(t *testing.T) {
	t.Parallel()

	disk, err := New(0, t.TempDir(), Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const n = 50
	var wg sync.WaitGroup
	lbas := make(chan uint64, n)
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			lba, err := disk.Append(fmt.Sprintf("key-%d", i), []byte(fmt.Sprintf("value-%d", i)))
			if err != nil {
				errs <- err
				return
			}
			lbas <- lba
		}()
	}
	wg.Wait()
	close(lbas)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	seen := make(map[uint64]bool)
	for lba := range lbas {
		if seen[lba] {
			t.Fatalf("duplicate lba allocated: %d", lba)
		}
		seen[lba] = true
	}
	if len(seen) != n {
		t.Fatalf("allocated %d unique LBAs, want %d", len(seen), n)
	}
}

func TestInjectedFailureDoesNotConsumeLBA(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	failing, err := New(0, dir, Config{FailRate: 1.0})
	if err != nil {
		t.Fatalf("New(failing) error = %v", err)
	}
	if _, err := failing.Append("k", []byte("v")); err == nil {
		t.Fatal("Append() unexpectedly succeeded with FailRate=1")
	}

	healthy, err := New(0, dir, Config{})
	if err != nil {
		t.Fatalf("New(healthy) error = %v", err)
	}
	lba, err := healthy.Append("k", []byte("v"))
	if err != nil {
		t.Fatalf("Append() after injected failure error = %v", err)
	}
	if lba != 0 {
		t.Fatalf("first successful append lba = %d, want 0", lba)
	}
}

func TestRecoverLBAIgnoresUnrelatedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("WriteFile(notes) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-number.blk"), []byte("ignore me"), 0644); err != nil {
		t.Fatalf("WriteFile(non numeric blk) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7.blk"), []byte("1:k\nold"), 0644); err != nil {
		t.Fatalf("WriteFile(7.blk) error = %v", err)
	}

	disk, err := New(0, dir, Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	lba, err := disk.Append("new", []byte("value"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if lba != 8 {
		t.Fatalf("Append() lba = %d, want 8", lba)
	}
}

func TestReadMalformedBlockReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0.blk"), []byte("missing newline header"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	disk, err := New(0, dir, Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := disk.Read(0); err == nil {
		t.Fatal("Read() unexpectedly succeeded on malformed block")
	}
}
