package bucketfill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// VersionDir describes a single migration directory found by Scan.
//
// PkgName is always "v<N>" — even if the on-disk folder is "1" without the
// prefix, callers (the entry-binary scaffolder) need a valid Go identifier.
type VersionDir struct {
	Version int
	Path    string // absolute path to the version directory
	PkgName string // e.g. "v1"
}

var versionFolderRE = regexp.MustCompile(`^v?(\d+)$`)

// Scan walks migrationDir and returns the discovered version directories in
// ascending version order. It enforces:
//   - each match contains both up.go and down.go
//   - the same version isn't represented twice (e.g. both "1/" and "v1/")
//   - non-matching entries are ignored (they're not migrations)
func Scan(migrationDir string) ([]VersionDir, error) {
	abs, err := filepath.Abs(migrationDir)
	if err != nil {
		return nil, fmt.Errorf("bucketfill: resolve %s: %w", migrationDir, err)
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("bucketfill: migrations directory %s: %w", abs, os.ErrNotExist)
		}
		return nil, fmt.Errorf("bucketfill: read %s: %w", abs, err)
	}

	seen := map[int]string{} // version -> raw folder name, to spot duplicates
	var dirs []VersionDir

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := versionFolderRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, err := strconv.Atoi(m[1])
		if err != nil || v <= 0 {
			continue
		}
		if existing, dup := seen[v]; dup {
			return nil, fmt.Errorf("bucketfill: duplicate folders for v%d: %q and %q", v, existing, e.Name())
		}
		seen[v] = e.Name()

		path := filepath.Join(abs, e.Name())
		if err := requireFile(path, "up.go"); err != nil {
			return nil, err
		}
		if err := requireFile(path, "down.go"); err != nil {
			return nil, err
		}
		dirs = append(dirs, VersionDir{
			Version: v,
			Path:    path,
			PkgName: fmt.Sprintf("v%d", v),
		})
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Version < dirs[j].Version })
	return dirs, nil
}

func requireFile(dir, name string) error {
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("bucketfill: missing %s in %s", name, dir)
		}
		return fmt.Errorf("bucketfill: stat %s: %w", p, err)
	}
	return nil
}

// NextVersion returns the next unused version number under migrationDir.
// Returns 1 if the directory is empty or missing.
func NextVersion(migrationDir string) (int, error) {
	dirs, err := Scan(migrationDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 1, nil
		}
		return 0, err
	}
	if len(dirs) == 0 {
		return 1, nil
	}
	return dirs[len(dirs)-1].Version + 1, nil
}

