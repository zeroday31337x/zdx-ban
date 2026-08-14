package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"zdx-ban/internal/memory"
)

func memoryCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("memory requires inspect, stats, or reset")
	}
	path := env("BAN_MEMORY_FILE", "memory/memory.jsonl")
	store, e := memory.OpenJSONL(path)
	if e != nil {
		return e
	}
	switch args[0] {
	case "inspect":
		records, e := store.List(context.Background())
		if e != nil {
			return e
		}
		sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
		for _, r := range records {
			fmt.Printf("%s %s %s %s\n", r.ID, r.Tier, r.Status, r.Title)
		}
		return nil
	case "stats":
		records, e := store.List(context.Background())
		if e != nil {
			return e
		}
		hash, _ := store.SnapshotHash(context.Background())
		tiers := map[memory.Tier]int{}
		for _, r := range records {
			tiers[r.Tier]++
		}
		fmt.Printf("records=%d foundation=%d domain=%d episodic=%d hash=%s path=%s\n", len(records), tiers[memory.Foundation], tiers[memory.Domain], tiers[memory.Episodic], hash, path)
		return nil
	case "reset":
		if len(args) < 2 || args[1] != "--confirm" {
			return fmt.Errorf("memory reset requires --confirm")
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		return store.Reset(context.Background())
	default:
		return fmt.Errorf("unknown memory command %q", args[0])
	}
}
