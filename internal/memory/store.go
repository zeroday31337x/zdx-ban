package memory

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"zdx-ban/internal/measurement"
)

type Store interface {
	Append(context.Context, Record) error
	Get(context.Context, string) (Record, bool, error)
	List(context.Context) ([]Record, error)
	Replace(context.Context, Record) error
	Reset(context.Context) error
	SnapshotHash(context.Context) (string, error)
}
type MemoryStore struct {
	mu      sync.RWMutex
	records map[string]Record
	order   []string
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{records: map[string]Record{}} }
func validateRecord(r Record) error {
	if r.ID == "" || r.SchemaVersion != SchemaVersion || r.Tier == "" || r.Kind == "" || r.CreatedAt.IsZero() {
		return errors.New("invalid memory record")
	}
	if r.Provenance.SourceClass == MemoryGuidance && r.Provenance.Independence == measurement.Independent {
		return errors.New("memory guidance cannot be current independent evidence")
	}
	return nil
}
func (s *MemoryStore) Append(_ context.Context, r Record) error {
	if err := validateRecord(r); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[r.ID]; ok {
		return fmt.Errorf("duplicate memory id %q", r.ID)
	}
	s.records[r.ID] = r
	s.order = append(s.order, r.ID)
	return nil
}
func (s *MemoryStore) Get(_ context.Context, id string) (Record, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[id]
	return r, ok, nil
}
func (s *MemoryStore) List(_ context.Context) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.records[id])
	}
	return out, nil
}
func (s *MemoryStore) Replace(_ context.Context, r Record) error {
	if err := validateRecord(r); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.records[r.ID]
	if !ok {
		return fmt.Errorf("unknown memory id")
	}
	if old.Tier == Foundation && (old.Content != r.Content || old.Title != r.Title) {
		return fmt.Errorf("foundation memory content requires explicit supersession")
	}
	s.records[r.ID] = r
	return nil
}
func (s *MemoryStore) Reset(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = map[string]Record{}
	s.order = nil
	return nil
}
func (s *MemoryStore) SnapshotHash(ctx context.Context) (string, error) {
	v, e := s.List(ctx)
	if e != nil {
		return "", e
	}
	return HashRecords(v)
}
func HashRecords(v []Record) (string, error) {
	copyv := append([]Record(nil), v...)
	sort.Slice(copyv, func(i, j int) bool { return copyv[i].ID < copyv[j].ID })
	b, e := json.Marshal(copyv)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type envelope struct {
	SchemaVersion, Checksum string
	Record                  Record
}
type JSONLStore struct {
	mu     sync.Mutex
	Path   string
	memory *MemoryStore
}

func OpenJSONL(path string) (*JSONLStore, error) {
	s := &JSONLStore{Path: path, memory: NewMemoryStore()}
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4<<20)
	line := 0
	for scan.Scan() {
		line++
		var env envelope
		if e = json.Unmarshal(scan.Bytes(), &env); e != nil {
			return nil, fmt.Errorf("memory corruption line %d: %w", line, e)
		}
		if env.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("unsupported memory schema %q", env.SchemaVersion)
		}
		b, _ := json.Marshal(env.Record)
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != env.Checksum {
			return nil, fmt.Errorf("memory checksum mismatch line %d", line)
		}
		if e = s.memory.Append(context.Background(), env.Record); e != nil {
			return nil, e
		}
	}
	if e = scan.Err(); e != nil {
		return nil, e
	}
	return s, nil
}
func (s *JSONLStore) Append(ctx context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.memory.Append(ctx, r); e != nil {
		return e
	}
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	env := envelope{SchemaVersion: SchemaVersion, Checksum: hex.EncodeToString(h[:]), Record: r}
	line, e := json.Marshal(env)
	if e != nil {
		return e
	}
	os.MkdirAll(filepath.Dir(s.Path), 0700)
	f, e := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(append(line, '\n')); e != nil {
		return e
	}
	return f.Sync()
}
func (s *JSONLStore) Get(c context.Context, id string) (Record, bool, error) {
	return s.memory.Get(c, id)
}
func (s *JSONLStore) List(c context.Context) ([]Record, error) { return s.memory.List(c) }
func (s *JSONLStore) Replace(ctx context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.memory.Replace(ctx, r); e != nil {
		return e
	}
	return s.rewrite(ctx)
}
func (s *JSONLStore) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.memory.Reset(ctx); e != nil {
		return e
	}
	return os.WriteFile(s.Path, nil, 0600)
}
func (s *JSONLStore) SnapshotHash(c context.Context) (string, error) { return s.memory.SnapshotHash(c) }
func (s *JSONLStore) rewrite(ctx context.Context) error {
	records, _ := s.memory.List(ctx)
	tmp, e := os.CreateTemp(filepath.Dir(s.Path), ".zdx-memory-*.tmp")
	if e != nil {
		return e
	}
	name := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	for _, r := range records {
		b, _ := json.Marshal(r)
		h := sha256.Sum256(b)
		line, _ := json.Marshal(envelope{SchemaVersion: SchemaVersion, Checksum: hex.EncodeToString(h[:]), Record: r})
		if _, e = tmp.Write(append(line, '\n')); e != nil {
			return e
		}
	}
	if e = tmp.Sync(); e != nil {
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, s.Path); e != nil {
		return e
	}
	ok = true
	return nil
}
func NewRecord(id string, tier Tier, kind Kind, title, content string, p Provenance) Record {
	now := time.Now().UTC()
	if !p.CreatedAt.IsZero() {
		now = p.CreatedAt.UTC()
	}
	return Record{ID: id, SchemaVersion: SchemaVersion, Tier: tier, Kind: kind, Title: title, Content: content, CreatedAt: now, UpdatedAt: now, Provenance: p, MeasurementAuthority: p.Authority, Status: Active, CorrelationGroup: p.CorrelationGroup}
}
