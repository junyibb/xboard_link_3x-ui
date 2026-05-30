package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	path string
	Data Data `json:"data"`
}

type Data struct {
	Traffic   map[string]TrafficSnapshot `json:"traffic"`
	UpdatedAt int64                      `json:"updated_at"`
}

type TrafficSnapshot struct {
	UserID int64  `json:"user_id"`
	Up     uint64 `json:"up"`
	Down   uint64 `json:"down"`
}

func Load(path string) (*Store, error) {
	store := &Store{
		path: path,
		Data: Data{
			Traffic: make(map[string]TrafficSnapshot),
		},
	}

	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, err
	}
	if len(body) == 0 {
		return store, nil
	}
	if err := json.Unmarshal(body, &store.Data); err != nil {
		return nil, err
	}
	if store.Data.Traffic == nil {
		store.Data.Traffic = make(map[string]TrafficSnapshot)
	}
	return store, nil
}

func (s *Store) Save() error {
	s.Data.UpdatedAt = time.Now().Unix()
	body, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
