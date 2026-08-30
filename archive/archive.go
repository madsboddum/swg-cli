// Package archive resolves data file paths across a directory of .tre archives
// and the loose files sitting next to them, the way the game client does.
package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/madsboddum/swg-cli/toc"
	"github.com/madsboddum/swg-cli/tre"
)

// ErrNoArchives reports a directory that holds no .tre archives.
var ErrNoArchives = errors.New("no .tre archives in directory")

// Source is one place a path was found: a .tre archive, or a loose file on
// disk next to the archives.
type Source struct {
	// Archive is the archive's filename, empty when the path is a loose file.
	Archive string
}

// Loose reports whether the path was found as a file on disk rather than
// inside an archive.
func (s Source) Loose() bool { return s.Archive == "" }

func (s Source) String() string {
	if s.Loose() {
		return "loose"
	}
	return s.Archive
}

// Stack is a directory of .tre archives plus the loose files beside them,
// indexed by path.
//
// A path can appear in several places. Loose files win over any archive, and
// among archives filename sort order decides with the last one winning, so
// patch_02.tre shadows patch_01.tre.
type Stack struct {
	dir string

	// names lists the archives in precedence order, lowest first.
	names []string
	// tres holds the archives read through their own index, tocs those read
	// through a client table of contents.
	tres map[string]*tre.ReadCloser
	tocs []*toc.Reader
	// byArchive maps an archive filename to the paths a table of contents says
	// it holds, in table order.
	byArchive map[string][]string
	// tocFiles maps those same paths to the table entry describing where in the
	// archive the bytes sit.
	tocFiles map[tocKey]*toc.File

	// sources maps a path to every place it was found, winner first.
	sources map[string][]Source
	paths   []string
}

// tocKey addresses one member of one archive, the way a Source names it.
type tocKey struct{ archive, path string }

// Open indexes every .tre archive in dir along with the loose files beside
// them. Archives carrying no index of their own are read through the client
// .toc files in the same directory. Everything opened stays open until Close.
func Open(dir string) (*Stack, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	s := &Stack{
		dir:       dir,
		tres:      make(map[string]*tre.ReadCloser),
		byArchive: make(map[string][]string),
		tocFiles:  make(map[tocKey]*toc.File),
		sources:   make(map[string][]Source),
	}

	var tables []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".tre":
			s.names = append(s.names, e.Name())
		case ".toc":
			tables = append(tables, e.Name())
		}
	}
	if len(s.names) == 0 {
		return nil, fmt.Errorf("%s: %w", dir, ErrNoArchives)
	}
	sort.Strings(s.names)
	sort.Strings(tables)

	if err := s.readTables(tables); err != nil {
		_ = s.Close()
		return nil, err
	}
	// Index from the highest priority archive down, so each path's source list
	// comes out in precedence order.
	for i := len(s.names) - 1; i >= 0; i-- {
		if err := s.indexArchive(s.names[i]); err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	if err := s.addLoose(); err != nil {
		_ = s.Close()
		return nil, err
	}

	s.paths = make([]string, 0, len(s.sources))
	for p := range s.sources {
		s.paths = append(s.paths, p)
	}
	sort.Strings(s.paths)
	return s, nil
}

// Close closes every archive and table of contents the stack opened.
func (s *Stack) Close() error {
	var err error
	for _, r := range s.tres {
		if cerr := r.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	for _, r := range s.tocs {
		if cerr := r.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}

// Dir is the directory the stack was opened from.
func (s *Stack) Dir() string { return s.dir }

// Archives lists the archive filenames in precedence order, lowest first.
func (s *Stack) Archives() []string { return s.names }

// Paths lists every path in the stack, sorted, each appearing once however
// many places it was found in.
func (s *Stack) Paths() []string { return s.paths }

// Sources returns every place path was found, winner first. It returns nil if
// the path is in neither the archives nor the loose files.
func (s *Stack) Sources(path string) []Source { return s.sources[path] }

// ReadFile returns the contents of path as the client would see them, reading
// from the winning source. It reports fs.ErrNotExist if the path is in neither
// the archives nor the loose files.
func (s *Stack) ReadFile(path string) ([]byte, error) {
	srcs := s.sources[path]
	if len(srcs) == 0 {
		return nil, fmt.Errorf("%s: %w", path, fs.ErrNotExist)
	}
	return s.ReadFileFrom(path, srcs[0])
}

// ReadFileFrom returns the contents of path as one particular source holds it,
// ignoring precedence.
func (s *Stack) ReadFileFrom(path string, src Source) ([]byte, error) {
	if src.Loose() {
		return os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(path)))
	}
	if r, ok := s.tres[src.Archive]; ok {
		return r.ReadFile(path)
	}
	if f, ok := s.tocFiles[tocKey{archive: src.Archive, path: path}]; ok {
		return f.Bytes()
	}
	return nil, fmt.Errorf("%s in %s: %w", path, src.Archive, fs.ErrNotExist)
}

// ArchivePaths lists the paths held by a single archive, sorted, ignoring
// precedence. It reports fs.ErrNotExist if the stack holds no such archive.
func (s *Stack) ArchivePaths(name string) ([]string, error) {
	names, ok := s.byArchive[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, fs.ErrNotExist)
	}
	out := make([]string, len(names))
	copy(out, names)
	sort.Strings(out)
	return slices.Compact(out), nil
}

// readTables indexes the client tables of contents, recording which paths each
// archive holds. A table may name archives that are not installed; those are
// skipped.
func (s *Stack) readTables(tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	// Tables name their archives as the client wrote them, which need not match
	// the case on disk.
	installed := make(map[string]string, len(s.names))
	for _, n := range s.names {
		installed[strings.ToLower(n)] = n
	}

	for _, t := range tables {
		r, err := toc.Open(filepath.Join(s.dir, t))
		if err != nil {
			return err
		}
		s.tocs = append(s.tocs, r)
		for _, f := range r.Files() {
			name, ok := installed[strings.ToLower(f.Archive)]
			if !ok {
				continue
			}
			s.byArchive[name] = append(s.byArchive[name], f.Name)
			// A repeat within one archive shadows the later entry; keep the first,
			// matching how the readers resolve duplicates.
			k := tocKey{archive: name, path: f.Name}
			if _, dup := s.tocFiles[k]; !dup {
				s.tocFiles[k] = f
			}
		}
	}
	return nil
}

// indexArchive records the paths one archive holds. Archives a table of
// contents already covered are taken from there; the rest are read through
// their own index.
func (s *Stack) indexArchive(name string) error {
	if paths, ok := s.byArchive[name]; ok {
		for _, p := range paths {
			s.add(p, Source{Archive: name})
		}
		return nil
	}

	r, err := tre.Open(filepath.Join(s.dir, name))
	if err != nil {
		if errors.Is(err, tre.ErrNoIndex) {
			return fmt.Errorf("%s: %w, and no .toc in %s indexes it", name, tre.ErrNoIndex, s.dir)
		}
		return err
	}
	s.tres[name] = r

	files := r.Files()
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Name)
		s.add(f.Name, Source{Archive: name})
	}
	s.byArchive[name] = paths
	return nil
}

// add records that path was found in src. Sources are added an archive at a
// time, so a repeat of the previous one is a duplicate entry in that archive.
func (s *Stack) add(path string, src Source) {
	existing := s.sources[path]
	if n := len(existing); n > 0 && existing[n-1] == src {
		return
	}
	s.sources[path] = append(existing, src)
}

// addLoose walks the folders beside the archives for files overriding them.
// Loose files take precedence over every archive, so each goes to the front of
// its list. Files sitting directly in the directory are the client's own —
// executables, configuration, the archives themselves — not game data, so only
// the folders are walked.
func (s *Stack) addLoose() error {
	root := os.DirFS(s.dir)
	return fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.Contains(p, "/") {
			return nil
		}
		s.sources[p] = append([]Source{{}}, s.sources[p]...)
		return nil
	})
}
