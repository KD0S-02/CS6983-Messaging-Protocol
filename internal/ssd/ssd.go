package ssd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

type SSD struct {
	ID      uint8
	dataDir string
	nextLBA atomic.Uint64
	cfg     Config
}

func New(id uint8, dataDir string, cfg Config) (*SSD, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("ssd-%d: mkdir %s: %w", id, dataDir, err)
	}
	s := &SSD{ID: id, dataDir: dataDir, cfg: cfg}
	s.recoverLBA()
	return s, nil
}

func (s *SSD) Append(key string, value []byte) (uint64, error) {
	if err := s.cfg.apply(); err != nil {
		return 0, err
	}

	lba := s.nextLBA.Add(1) - 1
	path := s.blockPath(lba)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return 0, fmt.Errorf("ssd-%d: open lba=%d: %w", s.ID, lba, err)
	}
	defer f.Close()

	// "<keylen>:<key>\n<value bytes>"
	if _, err := fmt.Fprintf(f, "%d:%s\n", len(key), key); err != nil {
		return 0, fmt.Errorf("ssd-%d: write header lba=%d: %w", s.ID, lba, err)
	}
	if _, err := f.Write(value); err != nil {
		return 0, fmt.Errorf("ssd-%d: write value lba=%d: %w", s.ID, lba, err)
	}
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("ssd-%d: fsync lba=%d: %w", s.ID, lba, err)
	}

	return lba, nil
}

func (s *SSD) Read(lba uint64) ([]byte, error) {
	if err := s.cfg.apply(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.blockPath(lba))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("ssd-%d: lba=%d: %w", s.ID, lba, ErrNotFound)
		}
		return nil, fmt.Errorf("ssd-%d: read lba=%d: %w", s.ID, lba, err)
	}

	// skip header line
	for i, b := range data {
		if b == '\n' {
			return data[i+1:], nil
		}
	}

	return nil, fmt.Errorf("ssd-%d: lba=%d: malformed block, no header newline", s.ID, lba)
}

func (s *SSD) blockPath(lba uint64) string {
	return filepath.Join(s.dataDir, fmt.Sprintf("%d.blk", lba))
}

func (s *SSD) recoverLBA() {
	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return
	}
	var max uint64
	for _, e := range entries {
		var lba uint64
		if _, err := fmt.Sscanf(e.Name(), "%d.blk", &lba); err == nil {
			if lba+1 > max {
				max = lba + 1
			}
		}
	}
	s.nextLBA.Store(max)
}
