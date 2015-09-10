package fastentity

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

var (
	MAX_ENTITY_LEN     = 30
	DEFAULT_GROUP_SIZE = 1000
)

const (
	LEFT  = 0
	RIGHT = 1
)

type Pair [2]int

type Store struct {
	Lookup map[string]*Group
	sync.RWMutex
}

type Group struct {
	Name     string
	Entities map[string][][]rune
	Max_len  int
	sync.RWMutex
}

// Pops the last element and adds the new element
// to the front of stack.
func shift(n Pair, s []Pair) (Pair, []Pair) {
	if len(s) == 0 {
		return Pair{}, append(s, n)
	}
	if len(s) == cap(s) {
		return s[0], append(s[1:], n)
	}
	return s[0], append(s, n)
}

// Create a new entity group structure
func Init(groups ...string) *Store {
	store := new(Store)
	store.Lookup = make(map[string]*Group, len(groups))
	for _, name := range groups {
		group := &Group{
			Name:     name,
			Entities: make(map[string][][]rune, DEFAULT_GROUP_SIZE),
		}
		store.Lookup[name] = group
	}
	return store
}

// Add a new entity to a particular group
func (store *Store) Add(name string, entities ...[]rune) {
	if store.Lookup == nil {
		panic("You need to initialize the store before adding to it...")
	}

	store.Lock()
	group, ok := store.Lookup[name]
	if !ok {
		group = &Group{
			Name:     name,
			Entities: make(map[string][][]rune, DEFAULT_GROUP_SIZE),
		}
		store.Lookup[name] = group
	}
	store.Unlock()

	group.Lock()
	for _, e := range entities {
		h := hash([]rune(e))
		group.Entities[h] = append(group.Entities[h], e)
		if len(e) > group.Max_len {
			group.Max_len = len(e)
		}
	}
	group.Unlock()
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
func (store *Store) FindAll(rs []rune) map[string][][]rune {
	result := make(map[string][][]rune, len(store.Lookup))
	for name, group := range store.Lookup {
		result[name] = group.Find(rs)
	}
	return result
}

/*
// Find all entities for all type keys in parallel (slower)
func (store *Store) FindAll(rs []rune) map[string][][]rune {
	result := make(map[string][][]rune, len(store.Lookup))
	var wg sync.WaitGroup
	for name, group := range store.Lookup {
		wg.Add(1)
		go func(name string, group *Group) {
			result[name] = group.Find(rs)
			wg.Done()
		}(name, group)
	}
	wg.Wait()
	return result
}
*/

/*
// Find all together (slower)
func (store *Store) FindAll(rs []rune) map[string][][]rune {
	groups := make([]*Group, 0, len(store.Lookup))
	for _, group := range store.Lookup {
		groups = append(groups, group)
	}
	store.RLock()
	results := _find(rs, groups...)
	store.RUnlock()
	return results
}
*/

// Find only the entities of a given type = "key"
func (group *Group) Find(rs []rune) [][]rune {
	group.RLock()
	ents := _find(rs, group)
	group.RUnlock()
	return ents[group.Name]
}

// Lock free find for use internally
func _find(rs []rune, groups ...*Group) map[string][][]rune {

	results := make(map[string][][]rune, len(groups))

	pairs := make([]Pair, 0, 20)
	start := 0
	prevspace := true // First char of sequence is legit
	thisspace := false
	var p1, p2 Pair
	for off, r := range rs {

		// What are we looking at?
		thisspace = unicode.IsPunct(r) || unicode.IsSpace(r)

		if prevspace && !thisspace {
			// Word is beginning at this rune
			start = off
		} else if thisspace && !prevspace {
			// Word is ending, shift the pairs stack
			_, pairs = shift(Pair{start, off}, pairs)

			// Run the stack, check for entities working backwards from the current position
			if len(pairs) > 1 {
				p2 = pairs[len(pairs)-1]
				for i := len(pairs) - 1; i >= 0; i-- {
					p1 = pairs[i]
					if p2[RIGHT]-p1[LEFT] > MAX_ENTITY_LEN {
						break // Too long or short, can ignore it
					}
					for _, group := range groups {
						if p2[RIGHT]-p1[LEFT] > group.Max_len {
							continue
						}
						if ents, ok := group.Entities[hash(rs[p1[LEFT]:p2[RIGHT]])]; ok {
							// We have at least one entity with this key
							for _, ent := range ents {
								if len(ent) != p2[RIGHT]-p1[LEFT] {
									break
								}
								match := true
								for i, r := range ent {
									if unicode.ToLower(r) != unicode.ToLower(rs[p1[LEFT]+i]) {
										match = false
										break
									}
								}
								if match {
									results[group.Name] = append(results[group.Name], rs[p1[LEFT]:p2[RIGHT]])
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

// Create a new store by loading entity files from a given directory. The format
// expected has the format "<GROUP>.entities.csv"
func Load(dir string) (*Store, error) {
	dir = strings.TrimRight(dir, "/")
	store := Init()
	reGroupFile, _ := regexp.Compile("^(.+).entities.csv")
	var wg sync.WaitGroup
	fileCount := 0
	if files, err := ioutil.ReadDir(dir); err == nil {
		for _, fileInfo := range files {
			if m := reGroupFile.FindStringSubmatch(fileInfo.Name()); m != nil {
				wg.Add(1)
				go func(filename string, group string) {
					if file, err := os.Open(filename); err == nil {
						fileCount++
						defer file.Close()
						reader := bufio.NewScanner(file)
						for reader.Scan() {
							store.Add(group, []rune(reader.Text()))
						}
					} else {
						fmt.Printf("Unable to load \"%s\" entity file: %s: %s\n", group, filename, err.Error())
					}
					wg.Done()
				}(fmt.Sprintf("%s/%s", dir, fileInfo.Name()), m[1])
			}
		}
	}
	wg.Wait()
	if fileCount == 0 {
		return store, errors.New("There are no entity files")
	}
	return store, nil
}

// Save the existing entities to disk. Each group becomes a file with the format
// the format "<GROUP>.entities.csv" in the dir specified.
func (store *Store) Save(dir string) error {
	store.RLock()
	defer store.RUnlock()
	dir = strings.TrimRight(dir, "/")
	for name, group := range store.Lookup {
		filename := fmt.Sprintf("%s/%s", dir, strings.Replace(name, "/", "_", -1)+".entities.csv")
		if file, err := os.Create(filename); err != nil {
			// Failed
			return err
		} else {
			w := bufio.NewWriter(file)
			for _, entities := range group.Entities {
				for _, entity := range entities {
					w.WriteString(string(entity) + "\n")
				}
			}
			w.Flush()
			file.Close()
		}
	}
	return nil
}
