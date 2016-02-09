// Package fastentity provides text sequence identification in documents.
package fastentity

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"unicode"
)

var (
	// Maximum entity length.  NB: currently entities can be added which are longer
	// but they will be ignored in the search.
	// TODO: Maybe also error/warn on import?
	MaxEntityLen = 30
	// Number of entities to initially allocate when creating a Group.
	DefaultGroupSize = 1000
)

const (
	left  = 0
	right = 1
)

type pair [2]int

// Store is a collection of groups of entities.
type Store struct {
	sync.RWMutex // protects groups

	groups map[string]*group
}

type Entity struct {
	Text   []rune
	Offset int
}

type group struct {
	sync.RWMutex

	name     string
	entities map[string][][]rune
	maxLen   int
}

// Pops the last element and adds the new element to the front of stack.
func shift(n pair, s []pair) (pair, []pair) {
	if len(s) == 0 {
		return pair{}, append(s, n)
	}
	if len(s) == cap(s) {
		return s[0], append(s[1:], n)
	}
	return s[0], append(s, n)
}

// New creates a new Store of entity groups using the provided names.
func New(groups ...string) *Store {
	s := &Store{
		groups: make(map[string]*group, len(groups)),
	}
	for _, name := range groups {
		g := &group{
			name:     name,
			entities: make(map[string][][]rune, DefaultGroupSize),
		}
		s.groups[name] = g
	}
	return s
}

// Add adjoins the entities to the group identified by name.
func (s *Store) Add(name string, entities ...[]rune) {
	s.Lock()
	g, ok := s.groups[name]
	if !ok {
		g = &group{
			name:     name,
			entities: make(map[string][][]rune, DefaultGroupSize),
		}
		s.groups[name] = g
	}
	s.Unlock()

	g.Lock()
	for _, e := range entities {
		h := hash([]rune(e))
		g.entities[h] = append(g.entities[h], e)
		if len(e) > g.maxLen {
			g.maxLen = len(e)
		}
	}
	g.Unlock()
}

func hash(rs []rune) string {
	if len(rs) > 2 {
		return fmt.Sprintf("%s%s%s%03d", string(unicode.ToLower(rs[0])), string(unicode.ToLower(rs[1])), string(unicode.ToLower(rs[2])), len(rs))
	}
	if len(rs) > 1 {
		return fmt.Sprintf("%s%s%03d", string(unicode.ToLower(rs[0])), string(unicode.ToLower(rs[1])), len(rs))
	}
	return fmt.Sprintf("%s%03d", string(unicode.ToLower(rs[0])), len(rs))
}

// FindAll searches the input returning a maping group name -> found entities.
func (s *Store) FindAll(rs []rune) map[string][]Entity {
	result := make(map[string][]Entity, len(s.groups))
	for s, g := range s.groups {
		result[s] = g.Find(rs)
	}
	return result
}

// Find only the entities of a given type = "key"
func (g *group) Find(rs []rune) []Entity {
	g.RLock()
	ents := find(rs, []*group{g})
	g.RUnlock()
	return ents[g.name]
}

// Lock free find for use internally
func find(rs []rune, groups []*group) map[string][]Entity {
	results := make(map[string][]Entity, len(groups))
	pairs := make([]pair, 0, 20)
	start := 0
	prevSpace := true // First char of sequence is legit
	space := false

	for off, r := range rs {
		// What are we looking at?
		space = unicode.IsPunct(r) || unicode.IsSpace(r)

		if prevSpace && !space {
			// Word is beginning at this rune
			start = off
		} else if space && !prevSpace {
			// Word is ending, shift the pairs stack
			_, pairs = shift(pair{start, off}, pairs)

			// Run the stack, check for entities working backwards from the current position
			if len(pairs) > 1 {
				p2 := pairs[len(pairs)-1]
				for i := len(pairs) - 1; i >= 0; i-- {
					p1 := pairs[i]
					if p2[right]-p1[left] > MaxEntityLen {
						break // Too long or short, can ignore it
					}
					for _, g := range groups {
						if p2[right]-p1[left] > g.maxLen {
							continue
						}
						if ents, ok := g.entities[hash(rs[p1[left]:p2[right]])]; ok {
							// We have at least one entity with this key
							for _, ent := range ents {
								if len(ent) != p2[right]-p1[left] {
									break
								}
								match := true
								for i, r := range ent {
									if unicode.ToLower(r) != unicode.ToLower(rs[p1[left]+i]) {
										match = false
										break
									}
								}
								if match {
									results[g.name] = append(results[g.name],
										Entity{
											Text:   rs[p1[left]:p2[right]],
											Offset: p1[left],
										},
									)
								}
							}
						}
					}
				}
			}
		}

		// Mark prevSpace for the next loop
		if space {
			prevSpace = true
		} else {
			prevSpace = false
		}
	}
	return results
}

var entityFileSuffix = ".entities.csv"

// FromDir creates a new Store by loading entity files from a given directory path. Any files
// contained in the directory with names matching <group>.entities.csv will be imported,
// and the entities added to the group <group>.
func FromDir(dir string) (*Store, error) {
	dir = strings.TrimRight(dir, "/")
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	s := New()
	var wg sync.WaitGroup
	count := struct {
		sync.Mutex
		n int
	}{}

	errCh := make(chan error, len(files))
	for _, stat := range files {
		if strings.HasSuffix(stat.Name(), entityFileSuffix) {
			wg.Add(1)
			go func(path string, group string) {
				defer wg.Done()
				f, err := os.Open(path)
				if err != nil {
					errCh <- fmt.Errorf("error opening %v: %v\n", path, err)
					return
				}
				defer f.Close()

				err = AddFromReader(f, s, group)
				if err != nil {
					errCh <- fmt.Errorf("error reading from %v: %v\n", path, err)
					return
				}
				count.Lock()
				count.n++
				count.Unlock()
			}(fmt.Sprintf("%s/%s", dir, stat.Name()), strings.TrimSuffix(stat.Name(), entityFileSuffix))
		}
	}
	wg.Wait()
	close(errCh)

	for e := range errCh {
		if err == nil {
			err = e
		}
	}
	if err != nil {
		return nil, err
	}

	if count.n == 0 {
		return nil, errors.New("no entity files found")
	}
	return s, nil
}

// AddFromReader adds entities to the store under the group name from the io.Reader.
func AddFromReader(r io.Reader, store *Store, name string) error {
	s := bufio.NewScanner(r)
	for s.Scan() {
		rt := []rune(s.Text())
		if len(rt) > 0 {
			store.Add(name, rt)
		}
	}
	return s.Err()
}

// Save writes the existing entities to disk under the given directory path (assumed
// to already exist). Each entity group becomes a file <group>.entities.csv.
func (s *Store) Save(dir string) error {
	s.RLock()
	defer s.RUnlock()

	dir = strings.TrimRight(dir, "/")
	for name, g := range s.groups {
		path := fmt.Sprintf("%s/%s", dir, strings.Replace(name, "/", "_", -1)+entityFileSuffix)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()

		for _, entities := range g.entities {
			for _, e := range entities {
				f.WriteString(string(e) + "\n")
			}
		}
		f.Close()
	}
	return nil
}
