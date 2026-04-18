package ssd

import (
    "bytes"
    "errors"
    "testing"
)

func TestAppendReadRoundTrip(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    disk, err := New(0, dir, Config{})
    if err != nil {
        t.Fatalf("New() error = %v", err)
    }

    want := []byte("hello world")
    lba, err := disk.Append("user:1", want)
    if err != nil {
        t.Fatalf("Append() error = %v", err)
    }

    got, err := disk.Read(lba)
    if err != nil {
        t.Fatalf("Read() error = %v", err)
    }
    if !bytes.Equal(got, want) {
        t.Fatalf("Read() = %q, want %q", got, want)
    }
}

func TestRecoverLBAPreservesSequence(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    disk, err := New(1, dir, Config{})
    if err != nil {
        t.Fatalf("New() error = %v", err)
    }

    if _, err := disk.Append("a", []byte("1")); err != nil {
        t.Fatalf("Append #1 error = %v", err)
    }
    if _, err := disk.Append("b", []byte("2")); err != nil {
        t.Fatalf("Append #2 error = %v", err)
    }

    reopened, err := New(1, dir, Config{})
    if err != nil {
        t.Fatalf("New(reopen) error = %v", err)
    }

    lba, err := reopened.Append("c", []byte("3"))
    if err != nil {
        t.Fatalf("Append #3 error = %v", err)
    }
    if lba != 2 {
        t.Fatalf("Append() after restart lba = %d, want 2", lba)
    }
}

func TestReadNotFound(t *testing.T) {
    t.Parallel()

    dir := t.TempDir()
    disk, err := New(2, dir, Config{})
    if err != nil {
        t.Fatalf("New() error = %v", err)
    }

    _, err = disk.Read(999)
    if err == nil {
        t.Fatal("Read() unexpectedly succeeded")
    }
    if !errors.Is(err, ErrNotFound) {
        t.Fatalf("Read() error = %v, want ErrNotFound", err)
    }
}