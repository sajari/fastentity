// Package fastentity provides text sequence identification in documents.
package fastentity

import (
	"bufio"
	"errors"
	"fmt"
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

type Store struct {
	Lookup map[string]*Group
	sync.RWMutex
}

type Group struct {
	Name     string
	Entities map[string][][]rune
	MaxLen   int
	sync.RWMutex
}

// Pops the last element and adds the new element
// to the front of stack.
func shift(n pair, s []pair) (pair, []pair) {
	if len(s) == 0 {
		return pair{}, append(s, n)
	}
	if len(s) == cap(s) {
		return s[0], append(s[1:], n)
	}
	return s[0], append(s, n)
}

// Create a new entity group structure
func New(groups ...string) *Store {
	s := &Store{
		Lookup: make(map[string]*Group, len(groups)),
	}
	for _, name := range groups {
		g := &Group{
			Name:     name,
			Entities: make(map[string][][]rune, DefaultGroupSize),
		}
		s.Lookup[name] = g
	}
	return s
}

// Add a new entity to a particular group
func (s *Store) Add(name string, entities ...[]rune) {
	if s.Lookup == nil {
		panic("You need to initialize the s before adding to it...")
	}

	s.Lock()
	g, ok := s.Lookup[name]
	if !ok {
		g = &Group{
			Name:     name,
			Entities: make(map[string][][]rune, DefaultGroupSize),
		}
		s.Lookup[name] = g
	}
	s.Unlock()

	g.Lock()
	for _, e := range entities {
		h := hash([]rune(e))
		g.Entities[h] = append(g.Entities[h], e)
		if len(e) > g.MaxLen {
			g.MaxLen = len(e)
		}
	}
	g.Unlock()
}

// Take the string and turn it into a hash
func hash(rs []rune) string {
	if len(rs) > 2 {
		return fmt.Sprintf("%s%s%s%03d", string(unicode.ToLower(rs[0])), string(unicode.ToLower(rs[1])), string(unicode.ToLower(rs[2])), len(rs))
	}
	if len(rs) > 1 {
		return fmt.Sprintf("%s%s%03d", string(unicode.ToLower(rs[0])), string(unicode.ToLower(rs[1])), len(rs))
	}
	return fmt.Sprintf("%s%03d", string(unicode.ToLower(rs[0])), len(rs))
}

// Find all entities for all type keys
func (s *Store) FindAll(rs []rune) map[string][][]rune {
	result := make(map[string][][]rune, len(s.Lookup))
	for name, g := range s.Lookup {
		result[name] = g.Find(rs)
	}
	return result
}

// Find only the entities of a given type = "key"
func (g *Group) Find(rs []rune) [][]rune {
	g.RLock()
	ents := find(rs, g)
	g.RUnlock()
	return ents[g.Name]
}

// Lock free find for use internally
func find(rs []rune, groups ...*Group) map[string][][]rune {
	results := make(map[string][][]rune, len(groups))
	pairs := make([]pair, 0, 20)
	start := 0
	prevspace := true // First char of sequence is legit
	thisspace := false

	for off, r := range rs {
		// What are we looking at?
		thisspace = unicode.IsPunct(r) || unicode.IsSpace(r)

		if prevspace && !thisspace {
			// Word is beginning at this rune
			start = off
		} else if thisspace && !prevspace {
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
					for _, group := range groups {
						if p2[right]-p1[left] > group.MaxLen {
							continue
						}
						if ents, ok := group.Entities[hash(rs[p1[left]:p2[right]])]; ok {
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
									results[group.Name] = append(results[group.Name], rs[p1[left]:p2[right]])
								}
							}
						}
					}
				}
			}
		}

		// Mark prevspace for the next loop
		if thisspace {
			prevspace = true
		} else {
			prevspace = false
		}
	}
	return results
}

type incr struct {
	sync.Mutex
	n int
}

func (i *incr) incr() {
	i.Lock()
	i.n++
	i.Unlock()
}

var entityFileSuffix = ".entities.csv"

// Create a new store by loading entity files from a given directory. The format
// expected has the format "<GROUP>.entities.csv"
func Load(dir string) (*Store, error) {
	dir = strings.TrimRight(dir, "/")
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	s := New()
	var wg sync.WaitGroup
	count := &incr{}
	for _, fileInfo := range files {
		if strings.HasSuffix(fileInfo.Name(), entityFileSuffix) {
			wg.Add(1)
			go func(filename string, group string) {
				defer wg.Done()
				f, err := os.Open(filename)
				if err != nil {
					// TODO: Remove this, return an error instead?
					fmt.Printf("Unable to load \"%s\" entity file: %s: %s\n", group, filename, err.Error())
					return
				}
				defer f.Close()

				count.incr()
				r := bufio.NewScanner(f)
				for r.Scan() {
					s.Add(group, []rune(r.Text()))
				}
			}(fmt.Sprintf("%s/%s", dir, fileInfo.Name()), strings.TrimSuffix(fileInfo.Name(), entityFileSuffix))
		}
	}
	wg.Wait()
	if count.n == 0 {
		return s, errors.New("There are no entity files")
	}
	return s, nil
}

// Save the existing entities to disk. Each group becomes a file with the format
// the format "<GROUP>.entities.csv" in the dir specified.
func (s *Store) Save(dir string) error {
	s.RLock()
	defer s.RUnlock()
	dir = strings.TrimRight(dir, "/")
	for name, group := range s.Lookup {
		filename := fmt.Sprintf("%s/%s", dir, strings.Replace(name, "/", "_", -1)+entityFileSuffix)
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		w := bufio.NewWriter(f)
		for _, entities := range group.Entities {
			for _, e := range entities {
				w.WriteString(string(e) + "\n")
			}
		}
		w.Flush()
		f.Close()
	}
	return nil
}
